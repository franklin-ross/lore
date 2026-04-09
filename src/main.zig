const std = @import("std");
const lore = @import("lore");

fn print(comptime fmt: []const u8, args: anytype) void {
    const stdout = std.fs.File.stdout();
    var buf: [8192]u8 = undefined;
    const slice = std.fmt.bufPrint(&buf, fmt, args) catch return;
    stdout.writeAll(slice) catch {};
}

fn write(bytes: []const u8) void {
    const stdout = std.fs.File.stdout();
    stdout.writeAll(bytes) catch {};
}

pub fn main() !void {
    var gpa: std.heap.GeneralPurposeAllocator(.{}) = .init;
    defer _ = gpa.deinit();
    const allocator = gpa.allocator();

    const args = try std.process.argsAlloc(allocator);
    defer std.process.argsFree(allocator, args);

    if (args.len < 2) {
        printUsage();
        return;
    }

    const cmd = args[1];
    const cmd_args = args[2..];

    var project = lore.config.findAndLoad(allocator) catch |err| {
        std.debug.print("Error: {}\n", .{err});
        return;
    };
    defer project.deinit();

    var world = try lore.parser.parse(allocator, project);
    defer world.deinit();

    if (std.mem.eql(u8, cmd, "list")) {
        cmdList(world);
    } else if (std.mem.eql(u8, cmd, "query")) {
        cmdQuery(world, cmd_args);
    } else if (std.mem.eql(u8, cmd, "refs")) {
        cmdRefs(world, cmd_args);
    } else if (std.mem.eql(u8, cmd, "search")) {
        cmdSearch(world, cmd_args);
    } else if (std.mem.eql(u8, cmd, "check")) {
        cmdCheck(world);
    } else {
        std.debug.print("Unknown command: {s}\n", .{cmd});
        printUsage();
    }
}

fn printUsage() void {
    write(
        \\Usage: lore <command> [args]
        \\
        \\Commands:
        \\  list              List all entities
        \\  query <name>      Show entity description and references
        \\  refs <name>       Show all references to an entity
        \\  search <text>     Full-text search across all files
        \\  check             Report undefined references
        \\
    );
}

fn cmdList(world: lore.parser.World) void {
    for (world.entities.items) |entity| {
        if (entity.entity_type) |t| {
            print("{s} ({s})\n", .{ entity.name, t });
        } else {
            print("{s}\n", .{entity.name});
        }
    }
}

fn cmdQuery(world: lore.parser.World, args: []const []const u8) void {
    if (args.len == 0) {
        std.debug.print("Usage: lore query <name>\n", .{});
        return;
    }

    const name = args[0];
    const entity = switch (world.findEntity(name)) {
        .found => |e| e,
        .not_found => {
            std.debug.print("Entity not found: {s}\n", .{name});
            return;
        },
        .ambiguous => |matches| {
            std.debug.print("\"{s}\" is ambiguous. Did you mean:\n", .{name});
            for (matches.items()) |e| {
                if (e.entity_type) |t| {
                    std.debug.print("  {s} ({s})\n", .{ e.name, t });
                } else {
                    std.debug.print("  {s}\n", .{e.name});
                }
            }
            return;
        },
    };

    if (entity.entity_type) |t| {
        print("# {s} ({s})\n\n", .{ entity.name, t });
    } else {
        print("# {s}\n\n", .{entity.name});
    }

    if (entity.aliases.items.len > 0) {
        write("Also known as: ");
        for (entity.aliases.items, 0..) |alias, i| {
            if (i > 0) write(", ");
            write(alias);
        }
        write("\n\n");
    }

    for (entity.descriptions.items) |desc| {
        print("{s}\n", .{desc.text});
        print("  — {s}:{d}\n\n", .{ desc.file, desc.line });
    }

    const refs = world.getReferences(entity.name);
    if (refs.items.len > 0) {
        write("Referenced by:\n");
        for (refs.items) |ref| {
            print("  {s} — {s}:{d}\n", .{ ref.source_entity orelse "(free text)", ref.file, ref.line });
        }
    }
}

fn cmdRefs(world: lore.parser.World, args: []const []const u8) void {
    if (args.len == 0) {
        std.debug.print("Usage: lore refs <name>\n", .{});
        return;
    }

    const name = args[0];
    const refs = world.getReferences(name);
    if (refs.items.len == 0) {
        print("No references to \"{s}\".\n", .{name});
        return;
    }

    for (refs.items) |ref| {
        print("{s}:{d} — {s}\n", .{ ref.file, ref.line, ref.context });
    }
}

fn cmdSearch(world: lore.parser.World, args: []const []const u8) void {
    if (args.len == 0) {
        std.debug.print("Usage: lore search <text>\n", .{});
        return;
    }

    const query = args[0];
    var results = world.search(query);
    defer results.deinit(world.allocator);

    if (results.items.len == 0) {
        print("No results for \"{s}\".\n", .{query});
        return;
    }

    for (results.items) |result| {
        print("{s}:{d}: {s}\n", .{ result.file, result.line, result.context });
    }
}

fn cmdCheck(world: lore.parser.World) void {
    var issues = world.check();
    defer issues.deinit(world.allocator);

    if (issues.items.len == 0) {
        write("No issues found.\n");
        return;
    }

    for (issues.items) |issue| {
        print("{s}:{d}: {s}\n", .{ issue.file, issue.line, issue.message });
    }
}
