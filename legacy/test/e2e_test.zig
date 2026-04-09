const std = @import("std");

// End-to-end tests that run the compiled lore binary as a subprocess.

const fixture_session =
    \\# Session 1
    \\
    \\We entered the town and met some locals.
    \\
    \\Gundren (character) | Gundren Rockseeker: A dwarf merchant.
    \\  Hired us to deliver supplies to Phandalin.
    \\
    \\Phandalin (location): A small frontier town. Gundren sent us here.
    \\
    \\Deliver Supplies (quest): Given by Gundren. Take supplies to Phandalin.
    \\
    \\Count Strahd von Zarovich (character) | Strahd: Vampire lord.
    \\  Lives at Castle Ravenloft. Doesn't like sunlight.
    \\
    \\Castle Ravenloft (location): Strahd's castle. Very spooky.
;

fn setupFixtures(allocator: std.mem.Allocator) !struct { dir: std.testing.TmpDir, root: []const u8 } {
    var tmp = std.testing.tmpDir(.{});

    var toml_file = try tmp.dir.createFile("lore.toml", .{});
    try toml_file.writeAll("files = [\"**/*.md\"]\n");
    toml_file.close();

    var session_file = try tmp.dir.createFile("session-01.md", .{});
    try session_file.writeAll(fixture_session);
    session_file.close();

    const root = try tmp.dir.realpathAlloc(allocator, ".");
    return .{ .dir = tmp, .root = root };
}

const RunResult = struct {
    stdout: []const u8,
    stderr: []const u8,
    term: std.process.Child.Term,
    allocator: std.mem.Allocator,

    fn deinit(self: *const RunResult) void {
        self.allocator.free(self.stdout);
        self.allocator.free(self.stderr);
    }
};

fn runLore(allocator: std.mem.Allocator, cwd: []const u8, args: []const []const u8) !RunResult {
    // Find the binary — built by `zig build` into zig-out/bin/lore
    // We need the absolute path since we change cwd
    const project_root = try std.fs.cwd().realpathAlloc(allocator, ".");
    defer allocator.free(project_root);
    const binary_path = try std.fs.path.join(allocator, &.{ project_root, "zig-out/bin/lore" });
    defer allocator.free(binary_path);

    var argv: std.ArrayList([]const u8) = .empty;
    defer argv.deinit(allocator);
    try argv.append(allocator, binary_path);
    for (args) |arg| {
        try argv.append(allocator, arg);
    }

    var child = std.process.Child.init(argv.items, allocator);
    child.cwd = cwd;
    child.stdout_behavior = .Pipe;
    child.stderr_behavior = .Pipe;

    try child.spawn();

    var stdout_buf: std.ArrayList(u8) = .empty;
    var stderr_buf: std.ArrayList(u8) = .empty;

    try child.collectOutput(allocator, &stdout_buf, &stderr_buf, 1024 * 1024);
    const term = try child.wait();

    return RunResult{
        .stdout = try stdout_buf.toOwnedSlice(allocator),
        .stderr = try stderr_buf.toOwnedSlice(allocator),
        .term = term,
        .allocator = allocator,
    };
}

test "e2e: lore list shows all entities" {
    const allocator = std.testing.allocator;
    var fixtures = try setupFixtures(allocator);
    defer {
        allocator.free(fixtures.root);
        fixtures.dir.cleanup();
    }

    const result = try runLore(allocator, fixtures.root, &.{"list"});
    defer result.deinit();

    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "Gundren") != null);
    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "Phandalin") != null);
    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "Deliver Supplies") != null);
    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "Count Strahd von Zarovich") != null);
    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "Castle Ravenloft") != null);
}

test "e2e: lore query shows entity details" {
    const allocator = std.testing.allocator;
    var fixtures = try setupFixtures(allocator);
    defer {
        allocator.free(fixtures.root);
        fixtures.dir.cleanup();
    }

    const result = try runLore(allocator, fixtures.root, &.{ "query", "Gundren" });
    defer result.deinit();

    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "Gundren") != null);
    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "character") != null);
    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "dwarf merchant") != null);
    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "Gundren Rockseeker") != null);
}

test "e2e: lore query by alias" {
    const allocator = std.testing.allocator;
    var fixtures = try setupFixtures(allocator);
    defer {
        allocator.free(fixtures.root);
        fixtures.dir.cleanup();
    }

    const result = try runLore(allocator, fixtures.root, &.{ "query", "Strahd" });
    defer result.deinit();

    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "Count Strahd von Zarovich") != null);
    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "Vampire lord") != null);
}

test "e2e: lore search finds text" {
    const allocator = std.testing.allocator;
    var fixtures = try setupFixtures(allocator);
    defer {
        allocator.free(fixtures.root);
        fixtures.dir.cleanup();
    }

    const result = try runLore(allocator, fixtures.root, &.{ "search", "sunlight" });
    defer result.deinit();

    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "sunlight") != null);
}

test "e2e: lore query unknown entity" {
    const allocator = std.testing.allocator;
    var fixtures = try setupFixtures(allocator);
    defer {
        allocator.free(fixtures.root);
        fixtures.dir.cleanup();
    }

    const result = try runLore(allocator, fixtures.root, &.{ "query", "Nonexistent" });
    defer result.deinit();

    try std.testing.expect(std.mem.indexOf(u8, result.stderr, "not found") != null);
}

test "e2e: lore with no args shows usage" {
    const allocator = std.testing.allocator;
    var fixtures = try setupFixtures(allocator);
    defer {
        allocator.free(fixtures.root);
        fixtures.dir.cleanup();
    }

    const result = try runLore(allocator, fixtures.root, &.{});
    defer result.deinit();

    try std.testing.expect(std.mem.indexOf(u8, result.stdout, "Usage:") != null);
}
