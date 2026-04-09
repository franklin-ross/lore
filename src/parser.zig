const std = @import("std");
const config = @import("config.zig");
const entity = @import("entity.zig");
const world_mod = @import("world.zig");
const refs = @import("refs.zig");

pub const Entity = entity.Entity;
pub const Description = entity.Description;
pub const Reference = entity.Reference;
pub const SearchResult = entity.SearchResult;
pub const Issue = entity.Issue;
pub const World = world_mod.World;

/// Parse all files in the project into a World.
/// Caller owns the returned World and must call deinit().
pub fn parse(allocator: std.mem.Allocator, project: config.Project) !World {
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
        try refs.findReferences(arena_alloc, allocator, &world, file.content, file.name);
    }

    return world;
}

/// Parse entity definitions from file content.
/// arena_alloc: for string data that lives as long as the World.
/// list_alloc: for ArrayList/HashMap internal buffers (freed individually).
pub fn parseEntities(arena_alloc: std.mem.Allocator, list_alloc: std.mem.Allocator, world: *World, content: []const u8, file: []const u8) !void {
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

            const desc_text = try arena_alloc.dupe(u8, desc_buf.items);
            const ent = try findOrCreateEntity(arena_alloc, list_alloc, world, header.name, header.entity_type);

            if (header.entity_type) |t| {
                if (ent.entity_type == null) {
                    ent.entity_type = t;
                }
            }

            for (header.aliases()) |alias| {
                var found = false;
                for (ent.aliases.items) |existing| {
                    if (std.ascii.eqlIgnoreCase(existing, alias)) {
                        found = true;
                        break;
                    }
                }
                if (!found) {
                    try ent.aliases.append(list_alloc, alias);
                }
            }

            if (desc_text.len > 0) {
                try ent.descriptions.append(list_alloc, .{
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

    var canonical: ?[]const u8 = null;
    var alias_count: usize = 0;
    var alias_buf: [32][]const u8 = undefined;

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
fn findOrCreateEntity(arena_alloc: std.mem.Allocator, list_alloc: std.mem.Allocator, world: *World, name: []const u8, etype: ?[]const u8) !*Entity {
    for (world.entities.items) |*ent| {
        const name_match = std.ascii.eqlIgnoreCase(ent.name, name) or entity.nameMatchesAlias(ent, name);
        if (name_match) {
            if (etype == null) return ent;
            if (ent.entity_type == null) return ent;
            if (std.ascii.eqlIgnoreCase(ent.entity_type.?, etype.?)) return ent;
        }
    }

    _ = arena_alloc;
    try world.entities.append(list_alloc, .{
        .name = name,
        .entity_type = null,
        .aliases = .empty,
        .descriptions = .empty,
    });

    return &world.entities.items[world.entities.items.len - 1];
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
