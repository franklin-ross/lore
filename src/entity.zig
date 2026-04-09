const std = @import("std");

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

pub const Disambiguation = struct {
    name: []const u8,
    entity_type: []const u8,
};

/// Parse "Name (type)" disambiguation syntax from a lookup string.
pub fn parseDisambiguation(input: []const u8) ?Disambiguation {
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

pub fn nameMatchesAlias(entity: *const Entity, name: []const u8) bool {
    for (entity.aliases.items) |alias| {
        if (std.ascii.eqlIgnoreCase(alias, name)) return true;
    }
    return false;
}

/// Case-insensitive substring search.
pub fn containsIgnoreCase(haystack: []const u8, needle: []const u8) bool {
    if (needle.len > haystack.len) return false;
    var i: usize = 0;
    while (i + needle.len <= haystack.len) : (i += 1) {
        if (std.ascii.eqlIgnoreCase(haystack[i .. i + needle.len], needle)) {
            return true;
        }
    }
    return false;
}

test "containsIgnoreCase" {
    try std.testing.expect(containsIgnoreCase("Sildar Hallwinter is a fighter", "sildar"));
    try std.testing.expect(containsIgnoreCase("We met STRAHD", "strahd"));
    try std.testing.expect(!containsIgnoreCase("hello", "world"));
}

test "parseDisambiguation" {
    const result = parseDisambiguation("Barovia (town)").?;
    try std.testing.expectEqualStrings("Barovia", result.name);
    try std.testing.expectEqualStrings("town", result.entity_type);

    try std.testing.expect(parseDisambiguation("just a name") == null);
    try std.testing.expect(parseDisambiguation("name (type) trailing") == null);
    try std.testing.expect(parseDisambiguation("()") == null);
}
