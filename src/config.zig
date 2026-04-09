const std = @import("std");
const zlob = @import("zlob");

const config_filename = "lore.toml";

pub const Config = struct {
    files: []const []const u8,
    ignore: []const []const u8,
    gitignore: bool = true,
};

/// A loaded project with its root path and config.
pub const Project = struct {
    allocator: std.mem.Allocator,
    root: []const u8,
    config: Config,
    file_paths: std.ArrayList([]const u8),

    pub fn deinit(self: *Project) void {
        for (self.file_paths.items) |p| {
            self.allocator.free(p);
        }
        self.file_paths.deinit(self.allocator);
        self.allocator.free(self.root);
        for (self.config.files) |f| self.allocator.free(f);
        self.allocator.free(self.config.files);
        for (self.config.ignore) |i| self.allocator.free(i);
        self.allocator.free(self.config.ignore);
    }
};

/// Walk up from the current directory to find lore.toml and load it.
pub fn findAndLoad(allocator: std.mem.Allocator) !Project {
    const root = try findRoot(allocator) orelse return error.NoLoreToml;
    errdefer allocator.free(root);

    const config_path = try std.fs.path.join(allocator, &.{ root, config_filename });
    defer allocator.free(config_path);

    const cfg = try parseConfigFile(allocator, config_path);
    errdefer {
        for (cfg.files) |f| allocator.free(f);
        allocator.free(cfg.files);
        for (cfg.ignore) |i| allocator.free(i);
        allocator.free(cfg.ignore);
    }

    const file_paths = try collectFiles(allocator, root, cfg);

    return Project{
        .allocator = allocator,
        .root = root,
        .config = cfg,
        .file_paths = file_paths,
    };
}

/// Walk up directories from cwd to find lore.toml.
fn findRoot(allocator: std.mem.Allocator) !?[]const u8 {
    var dir = try std.fs.cwd().realpathAlloc(allocator, ".");

    while (true) {
        const candidate = try std.fs.path.join(allocator, &.{ dir, config_filename });
        defer allocator.free(candidate);

        std.fs.cwd().access(candidate, .{}) catch {
            const parent = std.fs.path.dirname(dir) orelse {
                allocator.free(dir);
                return null;
            };
            if (std.mem.eql(u8, parent, dir)) {
                allocator.free(dir);
                return null;
            }
            const new_dir = try allocator.dupe(u8, parent);
            allocator.free(dir);
            dir = new_dir;
            continue;
        };

        return dir;
    }
}

/// Minimal TOML parser — handles only what we need:
///   files = ["**/*.md"]
///   ignore = ["archive"]
///
/// This is intentionally minimal. If the config grows more complex,
/// graduate to a proper TOML library.
fn parseConfigFile(allocator: std.mem.Allocator, path: []const u8) !Config {
    const content = std.fs.cwd().readFileAlloc(allocator, path, 1024 * 1024) catch {
        const default_files = try allocator.alloc([]const u8, 1);
        default_files[0] = try allocator.dupe(u8, "**/*.md");
        return Config{
            .files = default_files,
            .ignore = try allocator.alloc([]const u8, 0),
        };
    };
    defer allocator.free(content);

    var files: std.ArrayList([]const u8) = .empty;
    var ignore: std.ArrayList([]const u8) = .empty;

    var lines = std.mem.splitSequence(u8, content, "\n");
    while (lines.next()) |line| {
        const trimmed = std.mem.trim(u8, line, " \t\r");
        if (trimmed.len == 0 or trimmed[0] == '#') continue;

        if (parseStringArray(allocator, trimmed, "files")) |values| {
            for (values) |v| try files.append(allocator, v);
            allocator.free(values);
        } else if (parseStringArray(allocator, trimmed, "ignore")) |values| {
            for (values) |v| try ignore.append(allocator, v);
            allocator.free(values);
        }
    }

    if (files.items.len == 0) {
        try files.append(allocator, try allocator.dupe(u8, "**/*.md"));
    }

    return Config{
        .files = try files.toOwnedSlice(allocator),
        .ignore = try ignore.toOwnedSlice(allocator),
    };
}

/// Parse a line like: key = ["val1", "val2"]
fn parseStringArray(allocator: std.mem.Allocator, line: []const u8, key: []const u8) ?[]const []const u8 {
    if (!std.mem.startsWith(u8, line, key)) return null;
    const after_key = std.mem.trim(u8, line[key.len..], " \t");
    if (after_key.len == 0 or after_key[0] != '=') return null;
    const after_eq = std.mem.trim(u8, after_key[1..], " \t");
    if (after_eq.len == 0 or after_eq[0] != '[') return null;

    const close = std.mem.indexOf(u8, after_eq, "]") orelse return null;
    const inner = after_eq[1..close];

    var result: std.ArrayList([]const u8) = .empty;
    var items = std.mem.splitSequence(u8, inner, ",");
    while (items.next()) |item| {
        const trimmed = std.mem.trim(u8, item, " \t");
        if (trimmed.len >= 2 and trimmed[0] == '"' and trimmed[trimmed.len - 1] == '"') {
            result.append(allocator, allocator.dupe(u8, trimmed[1 .. trimmed.len - 1]) catch return null) catch return null;
        }
    }

    return result.toOwnedSlice(allocator) catch null;
}

/// Collect all files matching the config globs, sorted alphabetically.
pub fn collectFiles(allocator: std.mem.Allocator, root: []const u8, cfg: Config) !std.ArrayList([]const u8) {
    var paths: std.ArrayList([]const u8) = .empty;

    const abs_root = try std.fs.cwd().realpathAlloc(allocator, root);
    defer allocator.free(abs_root);

    var flags = zlob.ZlobFlags.recommended();
    flags.gitignore = cfg.gitignore;

    var seen = std.StringHashMap(void).init(allocator);
    defer seen.deinit();

    for (cfg.files) |pattern| {
        const maybe_result = zlob.matchAt(allocator, abs_root, pattern, flags) catch continue;
        var result = maybe_result orelse continue;
        defer result.deinit();

        var it = result.iterator();
        while (it.next()) |abs_path| {
            const rel_path = if (std.mem.startsWith(u8, abs_path, abs_root))
                std.mem.trimLeft(u8, abs_path[abs_root.len..], "/")
            else
                abs_path;

            if (seen.contains(rel_path)) continue;
            if (isIgnored(rel_path, cfg.ignore)) continue;

            const duped = try allocator.dupe(u8, rel_path);
            try paths.append(allocator, duped);
            try seen.put(duped, {});
        }
    }

    std.mem.sort([]const u8, paths.items, {}, struct {
        fn lessThan(_: void, a: []const u8, b: []const u8) bool {
            return std.mem.order(u8, a, b) == .lt;
        }
    }.lessThan);

    return paths;
}

fn isIgnored(path: []const u8, ignore_patterns: []const []const u8) bool {
    for (ignore_patterns) |pattern| {
        if (zlob.fnmatch.fnmatch(pattern, path, .{})) return true;
    }
    return false;
}


// Tests

test "collectFiles with zlob glob matching" {
    const allocator = std.testing.allocator;

    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();

    try tmp.dir.writeFile(.{ .sub_path = "session.md", .data = "# Session 1\n" });
    try tmp.dir.makeDir("notes");
    try tmp.dir.writeFile(.{ .sub_path = "notes/npcs.md", .data = "# NPCs\n" });
    try tmp.dir.writeFile(.{ .sub_path = "readme.txt", .data = "not markdown\n" });

    const root = try tmp.dir.realpathAlloc(allocator, ".");
    defer allocator.free(root);

    var result = try collectFiles(allocator, root, .{
        .files = &.{"**/*.md"},
        .ignore = &.{},
    });
    defer {
        for (result.items) |p| allocator.free(p);
        result.deinit(allocator);
    }

    try std.testing.expectEqual(@as(usize, 2), result.items.len);
    for (result.items) |path| {
        try std.testing.expect(std.mem.endsWith(u8, path, ".md"));
    }
}

test "collectFiles respects ignore patterns" {
    const allocator = std.testing.allocator;

    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();

    try tmp.dir.writeFile(.{ .sub_path = "session.md", .data = "# Session 1\n" });
    try tmp.dir.makeDir("archive");
    try tmp.dir.writeFile(.{ .sub_path = "archive/old.md", .data = "# Old\n" });

    const root = try tmp.dir.realpathAlloc(allocator, ".");
    defer allocator.free(root);

    var result = try collectFiles(allocator, root, .{
        .files = &.{"**/*.md"},
        .ignore = &.{"archive/**"},
    });
    defer {
        for (result.items) |p| allocator.free(p);
        result.deinit(allocator);
    }

    try std.testing.expectEqual(@as(usize, 1), result.items.len);
    try std.testing.expectEqualStrings("session.md", result.items[0]);
}

test "collectFiles with multiple file patterns via brace expansion" {
    const allocator = std.testing.allocator;

    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();

    try tmp.dir.writeFile(.{ .sub_path = "session.md", .data = "" });
    try tmp.dir.writeFile(.{ .sub_path = "world.lore", .data = "" });
    try tmp.dir.writeFile(.{ .sub_path = "notes.txt", .data = "" });

    const root = try tmp.dir.realpathAlloc(allocator, ".");
    defer allocator.free(root);

    var result = try collectFiles(allocator, root, .{
        .files = &.{ "**/*.md", "**/*.lore" },
        .ignore = &.{},
    });
    defer {
        for (result.items) |p| allocator.free(p);
        result.deinit(allocator);
    }

    try std.testing.expectEqual(@as(usize, 2), result.items.len);
    try std.testing.expectEqualStrings("session.md", result.items[0]);
    try std.testing.expectEqualStrings("world.lore", result.items[1]);
}

test "collectFiles with deeply nested paths" {
    const allocator = std.testing.allocator;

    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();

    try tmp.dir.makeDir("campaigns");
    try tmp.dir.makePath("campaigns/strahd/sessions");
    try tmp.dir.writeFile(.{ .sub_path = "campaigns/strahd/sessions/01.md", .data = "" });
    try tmp.dir.writeFile(.{ .sub_path = "campaigns/strahd/glossary.md", .data = "" });
    try tmp.dir.writeFile(.{ .sub_path = "campaigns/notes.md", .data = "" });

    const root = try tmp.dir.realpathAlloc(allocator, ".");
    defer allocator.free(root);

    var result = try collectFiles(allocator, root, .{
        .files = &.{"**/*.md"},
        .ignore = &.{},
    });
    defer {
        for (result.items) |p| allocator.free(p);
        result.deinit(allocator);
    }

    try std.testing.expectEqual(@as(usize, 3), result.items.len);
    // Sorted alphabetically
    try std.testing.expectEqualStrings("campaigns/notes.md", result.items[0]);
    try std.testing.expectEqualStrings("campaigns/strahd/glossary.md", result.items[1]);
    try std.testing.expectEqualStrings("campaigns/strahd/sessions/01.md", result.items[2]);
}

test "collectFiles ignores by extension glob" {
    const allocator = std.testing.allocator;

    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();

    try tmp.dir.writeFile(.{ .sub_path = "session.md", .data = "" });
    try tmp.dir.writeFile(.{ .sub_path = "backup.md.bak", .data = "" });
    try tmp.dir.writeFile(.{ .sub_path = "notes.md", .data = "" });

    const root = try tmp.dir.realpathAlloc(allocator, ".");
    defer allocator.free(root);

    var result = try collectFiles(allocator, root, .{
        .files = &.{"*"},
        .ignore = &.{"*.bak"},
    });
    defer {
        for (result.items) |p| allocator.free(p);
        result.deinit(allocator);
    }

    try std.testing.expectEqual(@as(usize, 2), result.items.len);
    for (result.items) |path| {
        try std.testing.expect(!std.mem.endsWith(u8, path, ".bak"));
    }
}

test "collectFiles with multiple ignore patterns" {
    const allocator = std.testing.allocator;

    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();

    try tmp.dir.writeFile(.{ .sub_path = "session.md", .data = "" });
    try tmp.dir.makeDir("archive");
    try tmp.dir.writeFile(.{ .sub_path = "archive/old.md", .data = "" });
    try tmp.dir.makeDir("drafts");
    try tmp.dir.writeFile(.{ .sub_path = "drafts/wip.md", .data = "" });
    try tmp.dir.writeFile(.{ .sub_path = "notes.md", .data = "" });

    const root = try tmp.dir.realpathAlloc(allocator, ".");
    defer allocator.free(root);

    var result = try collectFiles(allocator, root, .{
        .files = &.{"**/*.md"},
        .ignore = &.{ "archive/**", "drafts/**" },
    });
    defer {
        for (result.items) |p| allocator.free(p);
        result.deinit(allocator);
    }

    try std.testing.expectEqual(@as(usize, 2), result.items.len);
    try std.testing.expectEqualStrings("notes.md", result.items[0]);
    try std.testing.expectEqualStrings("session.md", result.items[1]);
}

test "collectFiles with no matching files returns empty" {
    const allocator = std.testing.allocator;

    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();

    try tmp.dir.writeFile(.{ .sub_path = "readme.txt", .data = "" });

    const root = try tmp.dir.realpathAlloc(allocator, ".");
    defer allocator.free(root);

    var result = try collectFiles(allocator, root, .{
        .files = &.{"**/*.md"},
        .ignore = &.{},
    });
    defer {
        for (result.items) |p| allocator.free(p);
        result.deinit(allocator);
    }

    try std.testing.expectEqual(@as(usize, 0), result.items.len);
}

test "collectFiles ignore pattern does not affect non-matching paths" {
    const allocator = std.testing.allocator;

    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();

    try tmp.dir.writeFile(.{ .sub_path = "session.md", .data = "" });
    try tmp.dir.writeFile(.{ .sub_path = "notes.md", .data = "" });

    const root = try tmp.dir.realpathAlloc(allocator, ".");
    defer allocator.free(root);

    // Ignore pattern that matches nothing present
    var result = try collectFiles(allocator, root, .{
        .files = &.{"**/*.md"},
        .ignore = &.{"archive/**"},
    });
    defer {
        for (result.items) |p| allocator.free(p);
        result.deinit(allocator);
    }

    try std.testing.expectEqual(@as(usize, 2), result.items.len);
}

test "parseStringArray" {
    const allocator = std.testing.allocator;

    const result = parseStringArray(allocator, "files = [\"**/*.md\", \"**/*.lore\"]", "files") orelse {
        return error.TestUnexpectedResult;
    };
    defer {
        for (result) |r| allocator.free(r);
        allocator.free(result);
    }

    try std.testing.expectEqual(@as(usize, 2), result.len);
    try std.testing.expectEqualStrings("**/*.md", result[0]);
    try std.testing.expectEqualStrings("**/*.lore", result[1]);
}

test "parseStringArray wrong key" {
    const allocator = std.testing.allocator;
    const result = parseStringArray(allocator, "files = [\"**/*.md\"]", "ignore");
    try std.testing.expect(result == null);
}
