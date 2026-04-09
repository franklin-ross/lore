const std = @import("std");
const config = @import("config.zig");
const entity = @import("entity.zig");
const parser = @import("parser.zig");
const refs = @import("refs.zig");
const world_mod = @import("world.zig");

const World = world_mod.World;
const Reference = entity.Reference;
const containsIgnoreCase = entity.containsIgnoreCase;

fn setupTestProject(allocator: std.mem.Allocator, files: []const struct { name: []const u8, content: []const u8 }) !config.Project {
    var tmp = std.testing.tmpDir(.{});

    var toml_file = try tmp.dir.createFile("lore.toml", .{});
    try toml_file.writeAll("files = [\"**/*.md\"]\n");
    toml_file.close();

    for (files) |f| {
        if (std.fs.path.dirname(f.name)) |dir| {
            tmp.dir.makePath(dir) catch {};
        }
        var file = try tmp.dir.createFile(f.name, .{});
        try file.writeAll(f.content);
        file.close();
    }

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

/// Create a World directly from content strings, without touching the filesystem.
fn setupTestWorld(allocator: std.mem.Allocator, content: []const u8) !World {
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

    const arena_alloc = world.arena.allocator();
    try parser.parseEntities(arena_alloc, allocator, &world, content, "test.md");
    try refs.findReferences(arena_alloc, allocator, &world, content, "test.md");

    return world;
}

// Integration tests — full parse pipeline with real files

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

    var world = try parser.parse(allocator, project);
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

    var world = try parser.parse(allocator, project);
    defer world.deinit();

    const e = world.findEntity("Strahd").get().?;
    try std.testing.expectEqualStrings("Count Strahd von Zarovich", e.name);
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

    var world = try parser.parse(allocator, project);
    defer world.deinit();

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

    var world = try parser.parse(allocator, project);
    defer world.deinit();

    const r = world.getReferences("Gundren");
    try std.testing.expect(r.items.len > 0);
}

test "parse search finds text in descriptions" {
    const allocator = std.testing.allocator;
    var project = try setupTestProject(allocator, &.{
        .{ .name = "test.md", .content =
        \\Gundren (character): A dwarf merchant from Neverwinter.
        },
    });
    defer project.deinit();

    var world = try parser.parse(allocator, project);
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

    var world = try parser.parse(allocator, project);
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

    var world = try parser.parse(allocator, project);
    defer world.deinit();

    try std.testing.expectEqualStrings("Gundren", world.entities.items[0].name);
    try std.testing.expectEqualStrings("Sildar", world.entities.items[1].name);
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

    var world = try parser.parse(allocator, project);
    defer world.deinit();

    try std.testing.expectEqual(@as(usize, 1), world.entities.items.len);
}

// Disambiguation tests

test "disambiguates entities with same name but different types" {
    const allocator = std.testing.allocator;
    var world = try setupTestWorld(allocator,
        \\Barovia (town): Gothic, dark, misty, rundown.
        \\
        \\Barovia (nation): Perpetually cloudy. Nobody can leave.
    );
    defer world.deinit();

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
    var world = try setupTestWorld(allocator,
        \\Barovia (town): The main town.
        \\
        \\Barovia (nation): The country.
        \\
        \\We entered Barovia (town) from the west.
    );
    defer world.deinit();

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
    var world = try setupTestWorld(allocator,
        \\Barovia (town): The main town.
        \\
        \\Barovia (nation): The country.
    );
    defer world.deinit();

    const result = world.findEntity("Barovia");
    try std.testing.expect(result == .ambiguous);
    try std.testing.expectEqual(@as(usize, 2), result.ambiguous.count);

    try std.testing.expect(world.findEntity("Barovia (town)") == .found);
    try std.testing.expect(world.findEntity("Barovia (nation)") == .found);

    try std.testing.expect(world.findEntity("Barovia (city)") == .not_found);
    try std.testing.expect(world.findEntity("Neverwinter") == .not_found);
}
