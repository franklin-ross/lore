const std = @import("std");
const config = @import("config.zig");

pub const Description = struct {
    text: []const u8,
    file: []const u8,
    line: usize,
};

pub const Reference = struct {
    file: []const u8,
    line: usize,
    source_entity: ?[]const u8,
    context: []const u8,
};

pub const SearchResult = struct {
    file: []const u8,
    line: usize,
    context: []const u8,
};

pub const Issue = struct {
    file: []const u8,
    line: usize,
    message: []const u8,
};

pub const Entity = struct {
    name: []const u8,
    entity_type: ?[]const u8,
    aliases: std.ArrayList([]const u8),
    descriptions: std.ArrayList(Description),
};

pub const World = struct {
    /// Arena owns all string data allocated during parsing. Heap-allocated
    /// to avoid invalidation when World is moved/returned.
    arena: *std.heap.ArenaAllocator,
    allocator: std.mem.Allocator,
    entities: std.ArrayList(Entity),
    references: std.StringHashMap(std.ArrayList(Reference)),

    pub fn deinit(self: *World) void {
        // Free each entity's nested ArrayLists (their buffers are GPA-owned)
        for (self.entities.items) |*entity| {
            entity.aliases.deinit(self.allocator);
            entity.descriptions.deinit(self.allocator);
        }
        self.entities.deinit(self.allocator);
        var it = self.references.iterator();
        while (it.next()) |entry| {
            entry.value_ptr.deinit(self.allocator);
        }
        self.references.deinit();
        // Free all arena memory (file content, duped strings, etc.)
        self.arena.deinit();
        self.allocator.destroy(self.arena);
    }

    const max_ambiguous = 8;

    pub const FindResult = union(enum) {
        found: *const Entity,
        not_found,
        ambiguous: struct {
            entities: [max_ambiguous]*const Entity,
            count: usize,

            pub fn items(self: *const @This()) []const *const Entity {
                return self.entities[0..self.count];
            }
        },

        /// Returns the entity if found, null otherwise.
        pub fn get(self: FindResult) ?*const Entity {
            return switch (self) {
                .found => |e| e,
                else => null,
            };
        }
    };

    pub fn findEntity(self: *const World, name: []const u8) FindResult {
        // Support disambiguation syntax: "Barovia (town)"
        if (parseDisambiguation(name)) |disambig| {
            for (self.entities.items) |*entity| {
                if (entity.entity_type) |et| {
                    if (!std.ascii.eqlIgnoreCase(et, disambig.entity_type)) continue;
                    if (std.ascii.eqlIgnoreCase(entity.name, disambig.name)) return .{ .found = entity };
                    if (nameMatchesAlias(entity, disambig.name)) return .{ .found = entity };
                }
            }
            return .not_found;
        }

        // Collect all matches to detect ambiguity
        var matches: [max_ambiguous]*const Entity = undefined;
        var match_count: usize = 0;

        for (self.entities.items) |*entity| {
            if (std.ascii.eqlIgnoreCase(entity.name, name) or nameMatchesAlias(entity, name)) {
                if (match_count < matches.len) {
                    matches[match_count] = entity;
                    match_count += 1;
                }
            }
        }

        return switch (match_count) {
            0 => .not_found,
            1 => .{ .found = matches[0] },
            else => .{ .ambiguous = .{ .entities = matches, .count = match_count } },
        };
    }

    pub fn getReferences(self: *const World, name: []const u8) std.ArrayList(Reference) {
        if (self.references.get(name)) |refs| return refs;
        var it = self.references.iterator();
        while (it.next()) |entry| {
            if (std.ascii.eqlIgnoreCase(entry.key_ptr.*, name)) return entry.value_ptr.*;
        }
        var empty: std.ArrayList(Reference) = .empty;
        _ = &empty;
        return empty;
    }

    pub fn search(self: *const World, query: []const u8) std.ArrayList(SearchResult) {
        var results: std.ArrayList(SearchResult) = .empty;
        for (self.entities.items) |entity| {
            for (entity.descriptions.items) |desc| {
                if (containsIgnoreCase(desc.text, query)) {
                    results.append(self.allocator, .{ // allocator for container metadata
                        .file = desc.file,
                        .line = desc.line,
                        .context = desc.text,
                    }) catch {};
                }
            }
        }
        return results;
    }

    pub fn check(self: *const World) std.ArrayList(Issue) {
        _ = self;
        var empty: std.ArrayList(Issue) = .empty;
        _ = &empty;
        return empty;
    }
};

/// Parse all files in the project into a World.
/// Caller owns the returned World and must call deinit().
pub fn parse(allocator: std.mem.Allocator, project: config.Project) !World {
    // Heap-allocate the arena so it doesn't move when World is returned.
    const arena_ptr = try allocator.create(std.heap.ArenaAllocator);
    arena_ptr.* = std.heap.ArenaAllocator.init(allocator);
    const arena_alloc = arena_ptr.allocator();
    var world = World{
        .arena = arena_ptr,
        .allocator = allocator,
        .entities = .empty,
        .references = std.StringHashMap(std.ArrayList(Reference)).init(allocator),
    };

    // Read all files once into arena — content lives as long as the World
    const FileData = struct { name: []const u8, content: []const u8 };
    var files: std.ArrayList(FileData) = .empty;
    defer files.deinit(allocator);

    for (project.file_paths.items) |rel_path| {
        const full_path = try std.fs.path.join(allocator, &.{ project.root, rel_path });
        defer allocator.free(full_path);

        const content = std.fs.cwd().readFileAlloc(arena_alloc, full_path, 10 * 1024 * 1024) catch continue;
        const file_name = try arena_alloc.dupe(u8, rel_path);
        try files.append(allocator, .{ .name = file_name, .content = content });
    }

    // First pass: find all entity definitions
    for (files.items) |file| {
        try parseEntities(arena_alloc, allocator, &world, file.content, file.name);
    }

    // Second pass: find references to known entities in all text
    for (files.items) |file| {
        try findReferences(arena_alloc, allocator, &world, file.content, file.name);
    }

    return world;
}

/// Parse entity definitions from file content.
/// arena_alloc: for string data that lives as long as the World.
/// list_alloc: for ArrayList/HashMap internal buffers (freed individually).
fn parseEntities(arena_alloc: std.mem.Allocator, list_alloc: std.mem.Allocator, world: *World, content: []const u8, file: []const u8) !void {
    var line_num: usize = 0;
    var lines = std.mem.splitSequence(u8, content, "\n");

    while (lines.next()) |line| {
        line_num += 1;
        const trimmed = std.mem.trim(u8, line, " \t\r");

        if (trimmed.len == 0) continue;
        if (trimmed[0] == '#') continue;

        if (parseEntityHeader(trimmed)) |header| {
            const header_line = line_num;

            // Collect description until blank line
            var desc_buf: std.ArrayList(u8) = .empty;
            defer desc_buf.deinit(list_alloc);

            if (header.description_start.len > 0) {
                try desc_buf.appendSlice(list_alloc, header.description_start);
            }

            while (lines.next()) |next_line| {
                line_num += 1;
                const next_trimmed = std.mem.trim(u8, next_line, " \t\r");
                if (next_trimmed.len == 0) break;
                if (desc_buf.items.len > 0) try desc_buf.append(list_alloc, ' ');
                try desc_buf.appendSlice(list_alloc, next_trimmed);
            }

            // All string data goes into the arena — freed when World is deinited
            const desc_text = try arena_alloc.dupe(u8, desc_buf.items);
            const entity = try findOrCreateEntity(arena_alloc, list_alloc, world, header.name, header.entity_type);

            if (header.entity_type) |t| {
                if (entity.entity_type == null) {
                    entity.entity_type = t; // points into arena-owned file content
                }
            }

            for (header.aliases()) |alias| {
                var found = false;
                for (entity.aliases.items) |existing| {
                    if (std.ascii.eqlIgnoreCase(existing, alias)) {
                        found = true;
                        break;
                    }
                }
                if (!found) {
                    try entity.aliases.append(list_alloc, alias); // points into arena-owned content
                }
            }

            if (desc_text.len > 0) {
                try entity.descriptions.append(list_alloc, .{
                    .text = desc_text,
                    .file = file,
                    .line = header_line,
                });
            }
        }
    }
}

const max_aliases = 16;

const EntityHeader = struct {
    name: []const u8,
    entity_type: ?[]const u8,
    alias_buf: [max_aliases][]const u8 = undefined,
    alias_count: usize = 0,
    description_start: []const u8,

    fn aliases(self: *const EntityHeader) []const []const u8 {
        return self.alias_buf[0..self.alias_count];
    }
};

/// Parse an entity header line. Returns null if not an entity definition.
/// Requires (type) annotation to distinguish from free text.
fn parseEntityHeader(line: []const u8) ?EntityHeader {
    const colon_pos = std.mem.indexOf(u8, line, ":") orelse return null;
    const header_part = line[0..colon_pos];
    const desc_start = std.mem.trim(u8, line[colon_pos + 1 ..], " \t");

    // Extract (type) from anywhere in the header
    var entity_type: ?[]const u8 = null;
    var before_type: []const u8 = "";
    var after_type: []const u8 = "";

    if (std.mem.indexOf(u8, header_part, "(")) |open| {
        if (std.mem.indexOfPos(u8, header_part, open, ")")) |close| {
            entity_type = std.mem.trim(u8, header_part[open + 1 .. close], " \t");
            before_type = header_part[0..open];
            if (close + 1 < header_part.len) {
                after_type = header_part[close + 1 ..];
            }
        }
    }

    if (entity_type == null) return null;

    // Split by | for name and aliases, working on both before_type and
    // after_type segments. All returned slices point into the original
    // line — no local buffer needed.
    var canonical: ?[]const u8 = null;
    var alias_count: usize = 0;
    var alias_buf: [32][]const u8 = undefined;

    // Process segments from before the (type)
    var segments = std.mem.splitSequence(u8, before_type, "|");
    while (segments.next()) |segment| {
        const trimmed = std.mem.trim(u8, segment, " \t");
        if (trimmed.len == 0) continue;
        if (canonical == null) {
            canonical = trimmed;
        } else if (alias_count < alias_buf.len) {
            alias_buf[alias_count] = trimmed;
            alias_count += 1;
        }
    }

    // Process segments from after the (type)
    if (after_type.len > 0) {
        var after_segments = std.mem.splitSequence(u8, after_type, "|");
        while (after_segments.next()) |segment| {
            const trimmed = std.mem.trim(u8, segment, " \t");
            if (trimmed.len == 0) continue;
            if (canonical == null) {
                canonical = trimmed;
            } else if (alias_count < alias_buf.len) {
                alias_buf[alias_count] = trimmed;
                alias_count += 1;
            }
        }
    }

    if (canonical == null) return null;

    var header = EntityHeader{
        .name = canonical.?,
        .entity_type = entity_type,
        .description_start = desc_start,
    };
    header.alias_count = alias_count;
    if (alias_count > 0) {
        @memcpy(header.alias_buf[0..alias_count], alias_buf[0..alias_count]);
    }
    return header;
}

/// Find an existing entity by name/alias, or create a new one.
/// When entity_type is provided, uses name+type as a composite key so that
/// entities with the same name but different types (e.g. Barovia (town) vs
/// Barovia (nation)) are kept separate.
fn findOrCreateEntity(arena_alloc: std.mem.Allocator, list_alloc: std.mem.Allocator, world: *World, name: []const u8, entity_type: ?[]const u8) !*Entity {
    for (world.entities.items) |*entity| {
        const name_match = std.ascii.eqlIgnoreCase(entity.name, name) or nameMatchesAlias(entity, name);
        if (name_match) {
            // No type on this definition — attach to existing entity with this name
            if (entity_type == null) return entity;
            // Entity has no type yet — claim it
            if (entity.entity_type == null) return entity;
            // Types match — same entity
            if (std.ascii.eqlIgnoreCase(entity.entity_type.?, entity_type.?)) return entity;
            // Name matches but type differs — different entity, keep looking
        }
    }

    _ = arena_alloc;
    try world.entities.append(list_alloc, .{
        .name = name, // points into arena-owned file content
        .entity_type = null,
        .aliases = .empty,
        .descriptions = .empty,
    });

    return &world.entities.items[world.entities.items.len - 1];
}

const Disambiguation = struct {
    name: []const u8,
    entity_type: []const u8,
};

/// Parse "Name (type)" disambiguation syntax from a lookup string.
fn parseDisambiguation(input: []const u8) ?Disambiguation {
    const open = std.mem.lastIndexOf(u8, input, "(") orelse return null;
    const close = std.mem.lastIndexOf(u8, input, ")") orelse return null;
    if (close <= open) return null;
    // Must end with the closing paren (possibly trailing whitespace)
    if (std.mem.trim(u8, input[close + 1 ..], " \t").len != 0) return null;
    const name = std.mem.trim(u8, input[0..open], " \t");
    const entity_type = std.mem.trim(u8, input[open + 1 .. close], " \t");
    if (name.len == 0 or entity_type.len == 0) return null;
    return .{ .name = name, .entity_type = entity_type };
}

fn nameMatchesAlias(entity: *const Entity, name: []const u8) bool {
    for (entity.aliases.items) |alias| {
        if (std.ascii.eqlIgnoreCase(alias, name)) return true;
    }
    return false;
}

/// Find references to known entities in file content.
/// Handles disambiguated references like "Barovia (town)" by checking for
/// "name (type)" patterns first, then falling back to plain name matching.
fn findReferences(_: std.mem.Allocator, list_alloc: std.mem.Allocator, world: *World, content: []const u8, file: []const u8) !void {
    var line_num: usize = 0;
    var lines = std.mem.splitSequence(u8, content, "\n");

    while (lines.next()) |line| {
        line_num += 1;
        const trimmed = std.mem.trim(u8, line, " \t\r");
        if (trimmed.len == 0) continue;

        // Track which entities were matched by disambiguated references
        // so we don't also match them with a bare name.
        var disambig_matched: [256]bool = .{false} ** 256;

        // First pass: check for disambiguated references "name (type)"
        for (world.entities.items, 0..) |entity, i| {
            if (i >= disambig_matched.len) break;
            if (entity.entity_type) |et| {
                if (containsDisambiguatedRef(trimmed, entity.name, et)) {
                    disambig_matched[i] = true;
                    try addReference(list_alloc, world, entity.name, .{
                        .file = file,
                        .line = line_num,
                        .source_entity = findEntityAtLine(world, file, line_num),
                        .context = trimmed,
                    });
                }
            }
        }

        // Second pass: plain name/alias matching for non-disambiguated refs
        for (world.entities.items, 0..) |entity, i| {
            if (i < disambig_matched.len and disambig_matched[i]) continue;

            var matched = false;
            if (containsIgnoreCase(trimmed, entity.name)) {
                // Skip if this name appears only as part of a disambiguated
                // reference (e.g. "Barovia" in "Barovia (town)")
                if (!isOnlyDisambiguated(trimmed, entity.name, world)) {
                    matched = true;
                }
            }
            if (!matched) {
                for (entity.aliases.items) |alias| {
                    if (containsIgnoreCase(trimmed, alias)) {
                        matched = true;
                        break;
                    }
                }
            }
            if (matched) {
                try addReference(list_alloc, world, entity.name, .{
                    .file = file,
                    .line = line_num,
                    .source_entity = findEntityAtLine(world, file, line_num),
                    .context = trimmed,
                });
            }
        }
    }
}

/// Check if text contains "name (type)" as a disambiguated reference.
fn containsDisambiguatedRef(haystack: []const u8, name: []const u8, entity_type: []const u8) bool {
    // Look for "name (type)" pattern
    // The pattern is: name + " (" + type + ")"
    var i: usize = 0;
    while (i + name.len <= haystack.len) : (i += 1) {
        if (!std.ascii.eqlIgnoreCase(haystack[i .. i + name.len], name)) continue;

        // Check for " (type)" after the name
        const after = i + name.len;
        if (after + 3 + entity_type.len > haystack.len) continue;
        if (haystack[after] != ' ' or haystack[after + 1] != '(') continue;
        const type_start = after + 2;
        const type_end = type_start + entity_type.len;
        if (type_end >= haystack.len) continue;
        if (!std.ascii.eqlIgnoreCase(haystack[type_start..type_end], entity_type)) continue;
        if (haystack[type_end] != ')') continue;
        return true;
    }
    return false;
}

/// Check if every occurrence of `name` in `text` is followed by a " (type)"
/// pattern for some known entity type, meaning there are no bare references.
fn isOnlyDisambiguated(text: []const u8, name: []const u8, world: *World) bool {
    var i: usize = 0;
    while (i + name.len <= text.len) : (i += 1) {
        if (!std.ascii.eqlIgnoreCase(text[i .. i + name.len], name)) continue;

        // Found a name match — check if it's followed by " (type)"
        const after = i + name.len;
        if (after + 2 < text.len and text[after] == ' ' and text[after + 1] == '(') {
            // Looks like disambiguation syntax — check if type matches any entity
            if (std.mem.indexOfPos(u8, text, after + 2, ")")) |close| {
                const candidate_type = text[after + 2 .. close];
                var is_entity_type = false;
                for (world.entities.items) |entity| {
                    if (entity.entity_type) |et| {
                        if (std.ascii.eqlIgnoreCase(et, candidate_type)) {
                            is_entity_type = true;
                            break;
                        }
                    }
                }
                if (is_entity_type) {
                    i += name.len - 1; // skip past this match
                    continue;
                }
            }
        }
        // Found a bare (non-disambiguated) occurrence
        return false;
    }
    return true;
}

fn addReference(list_alloc: std.mem.Allocator, world: *World, entity_name: []const u8, ref: Reference) !void {
    const result = try world.references.getOrPut(entity_name);
    if (!result.found_existing) {
        result.value_ptr.* = .empty;
    }
    try result.value_ptr.append(list_alloc, ref);
}

fn findEntityAtLine(world: *World, file: []const u8, line: usize) ?[]const u8 {
    for (world.entities.items) |entity| {
        for (entity.descriptions.items) |desc| {
            if (std.mem.eql(u8, desc.file, file) and desc.line == line) {
                return entity.name;
            }
        }
    }
    return null;
}

/// Case-insensitive substring search.
fn containsIgnoreCase(haystack: []const u8, needle: []const u8) bool {
    if (needle.len > haystack.len) return false;
    var i: usize = 0;
    while (i + needle.len <= haystack.len) : (i += 1) {
        if (std.ascii.eqlIgnoreCase(haystack[i .. i + needle.len], needle)) {
            return true;
        }
    }
    return false;
}

// Tests

test "parseEntityHeader with type and alias" {
    const header = parseEntityHeader("Sildar Hallwinter (character) | Sildar: Fighter.") orelse {
        return error.TestUnexpectedResult;
    };

    try std.testing.expectEqualStrings("Sildar Hallwinter", header.name);
    try std.testing.expectEqualStrings("character", header.entity_type.?);
    try std.testing.expectEqual(@as(usize, 1), header.alias_count);
    try std.testing.expectEqualStrings("Sildar", header.aliases()[0]);
    try std.testing.expectEqualStrings("Fighter.", header.description_start);
}

test "parseEntityHeader type at start" {
    const header = parseEntityHeader("(location) Cragmaw Hideout: North of trail.") orelse {
        return error.TestUnexpectedResult;
    };

    try std.testing.expectEqualStrings("Cragmaw Hideout", header.name);
    try std.testing.expectEqualStrings("location", header.entity_type.?);
}

test "parseEntityHeader type in middle" {
    const header = parseEntityHeader("Count Strahd (character) | Strahd: Vampire.") orelse {
        return error.TestUnexpectedResult;
    };

    try std.testing.expectEqualStrings("Count Strahd", header.name);
    try std.testing.expectEqualStrings("character", header.entity_type.?);
    try std.testing.expectEqual(@as(usize, 1), header.alias_count);
    try std.testing.expectEqualStrings("Strahd", header.aliases()[0]);
}

test "parseEntityHeader no type returns null" {
    const result = parseEntityHeader("Just some text with a colon: here");
    try std.testing.expect(result == null);
}

test "parseEntityHeader no colon returns null" {
    const result = parseEntityHeader("No colon here");
    try std.testing.expect(result == null);
}

test "containsIgnoreCase" {
    try std.testing.expect(containsIgnoreCase("Sildar Hallwinter is a fighter", "sildar"));
    try std.testing.expect(containsIgnoreCase("We met STRAHD", "strahd"));
    try std.testing.expect(!containsIgnoreCase("hello", "world"));
}

// Full parser integration tests — create real files and parse them.

fn setupTestProject(allocator: std.mem.Allocator, files: []const struct { name: []const u8, content: []const u8 }) !config.Project {
    var tmp = std.testing.tmpDir(.{});

    // Write lore.toml
    var toml_file = try tmp.dir.createFile("lore.toml", .{});
    try toml_file.writeAll("files = [\"**/*.md\"]\n");
    toml_file.close();

    // Write test files
    for (files) |f| {
        // Create parent dirs if needed
        if (std.fs.path.dirname(f.name)) |dir| {
            tmp.dir.makePath(dir) catch {};
        }
        var file = try tmp.dir.createFile(f.name, .{});
        try file.writeAll(f.content);
        file.close();
    }

    // Get the real path of the tmp dir
    const root = try tmp.dir.realpathAlloc(allocator, ".");

    // Heap-allocate config arrays so they outlive this function.
    // Project.deinit() frees these.
    const files_arr = try allocator.alloc([]const u8, 1);
    files_arr[0] = try allocator.dupe(u8, "**/*.md");
    const ignore_arr = try allocator.alloc([]const u8, 0);

    const cfg = config.Config{
        .files = files_arr,
        .ignore = ignore_arr,
    };

    const file_paths = try config.collectFiles(allocator, root, cfg);

    return config.Project{
        .allocator = allocator,
        .root = root,
        .config = cfg,
        .file_paths = file_paths,
    };
}

test "parse single file with entities" {
    const allocator = std.testing.allocator;
    var project = try setupTestProject(allocator, &.{
        .{ .name = "session.md", .content = 
        \\Gundren (character) | Gundren Rockseeker: A dwarf merchant.
        \\  Hired us to deliver supplies.
        \\
        \\Phandalin (location): A small frontier town.
        },
    });
    defer project.deinit();

    var world = try parse(allocator, project);
    defer world.deinit();

    try std.testing.expectEqual(@as(usize, 2), world.entities.items.len);

    const gundren = world.findEntity("Gundren").get().?;
    try std.testing.expectEqualStrings("Gundren", gundren.name);
    try std.testing.expectEqualStrings("character", gundren.entity_type.?);
    try std.testing.expectEqual(@as(usize, 1), gundren.aliases.items.len);
    try std.testing.expectEqualStrings("Gundren Rockseeker", gundren.aliases.items[0]);

    const phandalin = world.findEntity("Phandalin").get().?;
    try std.testing.expectEqualStrings("location", phandalin.entity_type.?);
}

test "parse entity lookup by alias" {
    const allocator = std.testing.allocator;
    var project = try setupTestProject(allocator, &.{
        .{ .name = "test.md", .content = 
        \\Count Strahd von Zarovich (character) | Strahd: Vampire lord.
        },
    });
    defer project.deinit();

    var world = try parse(allocator, project);
    defer world.deinit();

    // Find by alias
    const entity = world.findEntity("Strahd").get().?;
    try std.testing.expectEqualStrings("Count Strahd von Zarovich", entity.name);
}

test "parse multi-file entity accumulation" {
    const allocator = std.testing.allocator;
    var project = try setupTestProject(allocator, &.{
        .{ .name = "glossary.md", .content = 
        \\Sildar (character): Fighter.
        },
        .{ .name = "session.md", .content = 
        \\Sildar: Was captured at Cragmaw Hideout.
        },
    });
    defer project.deinit();

    var world = try parse(allocator, project);
    defer world.deinit();

    // Should be one entity with two descriptions
    try std.testing.expectEqual(@as(usize, 1), world.entities.items.len);
    const sildar = world.findEntity("Sildar").get().?;
    try std.testing.expectEqual(@as(usize, 2), sildar.descriptions.items.len);
}

test "parse detects cross-references" {
    const allocator = std.testing.allocator;
    var project = try setupTestProject(allocator, &.{
        .{ .name = "test.md", .content = 
        \\Gundren (character): A dwarf.
        \\
        \\Phandalin (location): Where Gundren sent us.
        },
    });
    defer project.deinit();

    var world = try parse(allocator, project);
    defer world.deinit();

    // Phandalin's description mentions Gundren
    const refs = world.getReferences("Gundren");
    try std.testing.expect(refs.items.len > 0);
}

test "parse search finds text in descriptions" {
    const allocator = std.testing.allocator;
    var project = try setupTestProject(allocator, &.{
        .{ .name = "test.md", .content = 
        \\Gundren (character): A dwarf merchant from Neverwinter.
        },
    });
    defer project.deinit();

    var world = try parse(allocator, project);
    defer world.deinit();

    var results = world.search("Neverwinter");
    defer results.deinit(allocator);
    try std.testing.expectEqual(@as(usize, 1), results.items.len);
}

test "parse blank line ends entity description" {
    const allocator = std.testing.allocator;
    var project = try setupTestProject(allocator, &.{
        .{ .name = "test.md", .content = 
        \\Gundren (character): A dwarf.
        \\  More about Gundren.
        \\
        \\This is free text, not part of Gundren's description.
        },
    });
    defer project.deinit();

    var world = try parse(allocator, project);
    defer world.deinit();

    const gundren = world.findEntity("Gundren").get().?;
    try std.testing.expectEqual(@as(usize, 1), gundren.descriptions.items.len);
    try std.testing.expect(containsIgnoreCase(gundren.descriptions.items[0].text, "More about"));
    try std.testing.expect(!containsIgnoreCase(gundren.descriptions.items[0].text, "free text"));
}

test "parse files sorted alphabetically" {
    const allocator = std.testing.allocator;
    var project = try setupTestProject(allocator, &.{
        .{ .name = "b-second.md", .content = 
        \\Sildar (character): From the second file.
        },
        .{ .name = "a-first.md", .content = 
        \\Gundren (character): From the first file.
        },
    });
    defer project.deinit();

    var world = try parse(allocator, project);
    defer world.deinit();

    // Gundren should be first because a-first.md sorts before b-second.md
    try std.testing.expectEqualStrings("Gundren", world.entities.items[0].name);
    try std.testing.expectEqualStrings("Sildar", world.entities.items[1].name);
}

test "disambiguates entities with same name but different types" {
    const allocator = std.testing.allocator;
    var world = World{
        .arena = blk: {
            const a = try allocator.create(std.heap.ArenaAllocator);
            a.* = std.heap.ArenaAllocator.init(allocator);
            break :blk a;
        },
        .allocator = allocator,
        .entities = .empty,
        .references = std.StringHashMap(std.ArrayList(Reference)).init(allocator),
    };
    defer world.deinit();

    const content =
        \\Barovia (town): Gothic, dark, misty, rundown.
        \\
        \\Barovia (nation): Perpetually cloudy. Nobody can leave.
    ;

    try parseEntities(world.arena.allocator(), allocator, &world, content, "test.md");

    // Should be two separate entities
    try std.testing.expectEqual(@as(usize, 2), world.entities.items.len);

    const town = world.findEntity("Barovia (town)").get().?;
    try std.testing.expectEqualStrings("town", town.entity_type.?);
    try std.testing.expect(containsIgnoreCase(town.descriptions.items[0].text, "Gothic"));

    const nation = world.findEntity("Barovia (nation)").get().?;
    try std.testing.expectEqualStrings("nation", nation.entity_type.?);
    try std.testing.expect(containsIgnoreCase(nation.descriptions.items[0].text, "cloudy"));
}

test "disambiguated references resolve to correct entity" {
    const allocator = std.testing.allocator;
    var world = World{
        .arena = blk: {
            const a = try allocator.create(std.heap.ArenaAllocator);
            a.* = std.heap.ArenaAllocator.init(allocator);
            break :blk a;
        },
        .allocator = allocator,
        .entities = .empty,
        .references = std.StringHashMap(std.ArrayList(Reference)).init(allocator),
    };
    defer world.deinit();

    const content =
        \\Barovia (town): The main town.
        \\
        \\Barovia (nation): The country.
        \\
        \\We entered Barovia (town) from the west.
    ;

    const arena_alloc = world.arena.allocator();
    try parseEntities(arena_alloc, allocator, &world, content, "test.md");
    try findReferences(arena_alloc, allocator, &world, content, "test.md");

    // The free text "We entered Barovia (town)" should reference the town, not the nation
    const town = world.findEntity("Barovia (town)").get().?;
    const town_refs = world.getReferences(town.name);

    var has_free_text_ref = false;
    for (town_refs.items) |ref| {
        if (containsIgnoreCase(ref.context, "entered")) {
            has_free_text_ref = true;
            break;
        }
    }
    try std.testing.expect(has_free_text_ref);
}

test "findEntity returns ambiguous for bare name with multiple types" {
    const allocator = std.testing.allocator;
    var world = World{
        .arena = blk: {
            const a = try allocator.create(std.heap.ArenaAllocator);
            a.* = std.heap.ArenaAllocator.init(allocator);
            break :blk a;
        },
        .allocator = allocator,
        .entities = .empty,
        .references = std.StringHashMap(std.ArrayList(Reference)).init(allocator),
    };
    defer world.deinit();

    const content =
        \\Barovia (town): The main town.
        \\
        \\Barovia (nation): The country.
    ;

    try parseEntities(world.arena.allocator(), allocator, &world, content, "test.md");

    // Bare "Barovia" should be ambiguous
    const result = world.findEntity("Barovia");
    try std.testing.expect(result == .ambiguous);
    try std.testing.expectEqual(@as(usize, 2), result.ambiguous.count);

    // Disambiguated lookups should still work
    try std.testing.expect(world.findEntity("Barovia (town)") == .found);
    try std.testing.expect(world.findEntity("Barovia (nation)") == .found);

    // Non-existent should be not_found
    try std.testing.expect(world.findEntity("Barovia (city)") == .not_found);
    try std.testing.expect(world.findEntity("Neverwinter") == .not_found);
}

test "parse markdown headers are ignored" {
    const allocator = std.testing.allocator;
    var project = try setupTestProject(allocator, &.{
        .{ .name = "test.md", .content = 
        \\# Session 1
        \\
        \\Gundren (character): A dwarf.
        },
    });
    defer project.deinit();

    var world = try parse(allocator, project);
    defer world.deinit();

    // Should only have Gundren, not "# Session 1"
    try std.testing.expectEqual(@as(usize, 1), world.entities.items.len);
}
