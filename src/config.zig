const std = @import("std");

const config_filename = "lore.toml";

pub const Config = struct {
    files: []const []const u8,
    ignore: []const []const u8,
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

    var dir = try std.fs.cwd().openDir(root, .{ .iterate = true });
    defer dir.close();

    var walker = try dir.walk(allocator);
    defer walker.deinit();

    while (try walker.next()) |entry| {
        if (entry.kind != .file) continue;

        var ignored = false;
        for (cfg.ignore) |pattern| {
            if (std.mem.indexOf(u8, entry.path, pattern) != null) {
                ignored = true;
                break;
            }
        }
        if (ignored) continue;

        var matched = false;
        for (cfg.files) |pattern| {
            if (matchGlob(pattern, entry.path)) {
                matched = true;
                break;
            }
        }
        if (!matched) continue;

        try paths.append(allocator, try allocator.dupe(u8, entry.path));
    }

    std.mem.sort([]const u8, paths.items, {}, struct {
        fn lessThan(_: void, a: []const u8, b: []const u8) bool {
            return std.mem.order(u8, a, b) == .lt;
        }
    }.lessThan);

    return paths;
}

/// Simple glob matcher supporting:
///   **/*.ext — matches files with given extension in any subdirectory
///   *.ext    — matches files with given extension in current directory
fn matchGlob(pattern: []const u8, path: []const u8) bool {
    if (std.mem.startsWith(u8, pattern, "**/")) {
        const suffix_pattern = pattern[3..];
        if (suffix_pattern.len > 0 and suffix_pattern[0] == '*') {
            const ext = suffix_pattern[1..];
            return std.mem.endsWith(u8, path, ext);
        }
        const basename = std.fs.path.basename(path);
        return matchSimple(suffix_pattern, basename);
    }
    return matchSimple(pattern, path);
}

fn matchSimple(pattern: []const u8, str: []const u8) bool {
    if (std.mem.indexOf(u8, pattern, "*")) |star_pos| {
        const prefix = pattern[0..star_pos];
        const suffix = pattern[star_pos + 1 ..];
        return std.mem.startsWith(u8, str, prefix) and std.mem.endsWith(u8, str, suffix);
    }
    return std.mem.eql(u8, pattern, str);
}

// Tests

test "matchGlob basic patterns" {
    try std.testing.expect(matchGlob("**/*.md", "session-01.md"));
    try std.testing.expect(matchGlob("**/*.md", "sessions/01.md"));
    try std.testing.expect(matchGlob("**/*.md", "deep/nested/path/file.md"));
    try std.testing.expect(!matchGlob("**/*.md", "file.txt"));
    try std.testing.expect(!matchGlob("**/*.md", "file.markdown"));
}

test "matchGlob simple patterns" {
    try std.testing.expect(matchGlob("*.md", "file.md"));
    try std.testing.expect(!matchGlob("*.md", "dir/file.md"));
    try std.testing.expect(matchGlob("sessions/*", "sessions/01.md"));
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
