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

    pub fn findEntity(self: *const World, name: []const u8) ?*const Entity {
        for (self.entities.items) |*entity| {
            if (std.ascii.eqlIgnoreCase(entity.name, name)) return entity;
            for (entity.aliases.items) |alias| {
                if (std.ascii.eqlIgnoreCase(alias, name)) return entity;
            }
        }
        return null;
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
                    results.append(self.allocator, .{  // allocator for container metadata
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
            const entity = try findOrCreateEntity(arena_alloc, list_alloc, world, header.name);

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
fn findOrCreateEntity(arena_alloc: std.mem.Allocator, list_alloc: std.mem.Allocator, world: *World, name: []const u8) !*Entity {
    for (world.entities.items) |*entity| {
        if (std.ascii.eqlIgnoreCase(entity.name, name)) return entity;
        for (entity.aliases.items) |alias| {
            if (std.ascii.eqlIgnoreCase(alias, name)) return entity;
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

/// Find references to known entities in file content.
fn findReferences(_: std.mem.Allocator, list_alloc: std.mem.Allocator, world: *World, content: []const u8, file: []const u8) !void {
    var line_num: usize = 0;
    var lines = std.mem.splitSequence(u8, content, "\n");

    while (lines.next()) |line| {
        line_num += 1;
        const trimmed = std.mem.trim(u8, line, " \t\r");
        if (trimmed.len == 0) continue;

        for (world.entities.items) |entity| {
            var matched = false;
            if (containsIgnoreCase(trimmed, entity.name)) {
                matched = true;
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
                // trimmed points into arena-owned content, safe to keep
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
