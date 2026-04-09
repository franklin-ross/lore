const std = @import("std");
const entity = @import("entity.zig");
const world_mod = @import("world.zig");

const Entity = entity.Entity;
const Reference = entity.Reference;
const World = world_mod.World;

/// Find references to known entities in file content.
/// Handles disambiguated references like "Barovia (town)" by checking for
/// "name (type)" patterns first, then falling back to plain name matching.
pub fn findReferences(_: std.mem.Allocator, list_alloc: std.mem.Allocator, world: *World, content: []const u8, file: []const u8) !void {
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
        for (world.entities.items, 0..) |e, i| {
            if (i >= disambig_matched.len) break;
            if (e.entity_type) |et| {
                if (containsDisambiguatedRef(trimmed, e.name, et)) {
                    disambig_matched[i] = true;
                    try addReference(list_alloc, world, e.name, .{
                        .file = file,
                        .line = line_num,
                        .source_entity = findEntityAtLine(world, file, line_num),
                        .context = trimmed,
                    });
                }
            }
        }

        // Second pass: plain name/alias matching for non-disambiguated refs
        for (world.entities.items, 0..) |e, i| {
            if (i < disambig_matched.len and disambig_matched[i]) continue;

            var matched = false;
            if (entity.containsIgnoreCase(trimmed, e.name)) {
                // Skip if this name appears only as part of a disambiguated
                // reference (e.g. "Barovia" in "Barovia (town)")
                if (!isOnlyDisambiguated(trimmed, e.name, world)) {
                    matched = true;
                }
            }
            if (!matched) {
                for (e.aliases.items) |alias| {
                    if (entity.containsIgnoreCase(trimmed, alias)) {
                        matched = true;
                        break;
                    }
                }
            }
            if (matched) {
                try addReference(list_alloc, world, e.name, .{
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
    var i: usize = 0;
    while (i + name.len <= haystack.len) : (i += 1) {
        if (!std.ascii.eqlIgnoreCase(haystack[i .. i + name.len], name)) continue;

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

        const after = i + name.len;
        if (after + 2 < text.len and text[after] == ' ' and text[after + 1] == '(') {
            if (std.mem.indexOfPos(u8, text, after + 2, ")")) |close| {
                const candidate_type = text[after + 2 .. close];
                var is_entity_type = false;
                for (world.entities.items) |e| {
                    if (e.entity_type) |et| {
                        if (std.ascii.eqlIgnoreCase(et, candidate_type)) {
                            is_entity_type = true;
                            break;
                        }
                    }
                }
                if (is_entity_type) {
                    i += name.len - 1;
                    continue;
                }
            }
        }
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
    for (world.entities.items) |e| {
        for (e.descriptions.items) |desc| {
            if (std.mem.eql(u8, desc.file, file) and desc.line == line) {
                return e.name;
            }
        }
    }
    return null;
}
