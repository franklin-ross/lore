# Zig 0.15 Practical API Reference

Quick reference for current (0.15.x) APIs. Covers the major breaking changes from 0.13/0.14.

## ArrayList (Now Unmanaged by Default)

`std.ArrayList(T)` no longer stores an allocator. Pass it to every method call.

```zig
const std = @import("std");

var list: std.ArrayList(u8) = .empty;
defer list.deinit(allocator);

try list.append(allocator, 42);
try list.appendSlice(allocator, &.{ 1, 2, 3 });
const owned = try list.toOwnedSlice(allocator);
defer allocator.free(owned);
```

## HashMap / StringHashMap (Managed — Stores Allocator)

StringHashMap and AutoHashMap remain **managed**: pass allocator at init, not per-call.

```zig
var map = std.StringHashMap(i32).init(allocator);
defer map.deinit();

try map.put("key", 10);

const gop = try map.getOrPut("key");
if (!gop.found_existing) {
    gop.value_ptr.* = 99;
}

if (map.get("key")) |val| {
    std.debug.print("{}\n", .{val});
}
```

For the **unmanaged** variant (`StringHashMapUnmanaged`), pass allocator to `put`, `getOrPut`, `deinit`, etc. — same pattern as ArrayList.

## File I/O — Stdout (Writergate)

`std.io.getStdOut()` is gone. Writers now require an explicit buffer and a manual flush.

```zig
const std = @import("std");

pub fn main() !void {
    var buf: [4096]u8 = undefined;
    var stdout = std.fs.File.stdout().writer(&buf);
    try stdout.interface.print("Hello, {s}!\n", .{"world"});
    try stdout.interface.flush();
}
```

**Unbuffered** (writes go straight to the OS): pass an empty slice.

```zig
var stdout = std.fs.File.stdout().writer(&.{});
try stdout.interface.print("immediate\n", .{});
```

## ArenaAllocator

ArenaAllocator is a **pinned** type in 0.15 — you must not copy or move it after creation. Do not return it by value from a function or assign it to a new variable after init.

```zig
var arena = std.heap.ArenaAllocator.init(std.heap.page_allocator);
defer arena.deinit();
const alloc = arena.allocator();

const buf = try alloc.alloc(u8, 256);
// No need to free individual allocations; arena.deinit() frees everything.
```

To store in a struct, use a pointer or initialise in-place:

```zig
const MyThing = struct {
    arena: std.heap.ArenaAllocator,

    fn init(backing: std.mem.Allocator) MyThing {
        return .{ .arena = std.heap.ArenaAllocator.init(backing) };
    }
};
```

## std.fmt

`bufPrint` and `print` are unchanged in signature. Format string is comptime.

```zig
var buf: [128]u8 = undefined;
const s = try std.fmt.bufPrint(&buf, "{d} + {d} = {d}", .{ 1, 2, 3 });

// comptimePrint for comptime-known strings:
const msg = std.fmt.comptimePrint("version {d}", .{15});
```

## std.mem — Split, Tokenise, IndexOf

The iterator-based API has been stable since 0.12. Use `splitSequence`, `splitAny`, `tokenizeScalar`, `tokenizeAny`.

```zig
// Split on a sequence of bytes:
var it = std.mem.splitSequence(u8, "one::two::three", "::");
while (it.next()) |part| { _ = part; }

// Tokenise (skip consecutive delimiters):
var tok = std.mem.tokenizeScalar(u8, "  hello   world  ", ' ');
while (tok.next()) |word| { _ = word; }

// indexOf:
const idx = std.mem.indexOf(u8, "hello world", "world"); // returns ?usize
```

## Build System (build.zig)

0.15 requires `root_module` via `b.createModule()`. `addStaticLibrary` is replaced by `addLibrary`.

```zig
pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const exe = b.addExecutable(.{
        .name = "myapp",
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/main.zig"),
            .target = target,
            .optimize = optimize,
        }),
    });
    b.installArtifact(exe);

    const run_cmd = b.addRunArtifact(exe);
    const run_step = b.step("run", "Run the application");
    run_step.dependOn(&run_cmd.step);
}
```

## Testing

`std.testing.allocator` detects leaks. `std.testing.expect` is the primary assertion.

```zig
const std = @import("std");
const expect = std.testing.expect;

test "arraylist round-trip" {
    const allocator = std.testing.allocator;
    var list: std.ArrayList(u8) = .empty;
    defer list.deinit(allocator);

    try list.appendSlice(allocator, "hello");
    try expect(std.mem.eql(u8, list.items, "hello"));
}
```

## String Handling

Strings are `[]const u8`. No stdlib string type. Key patterns:

- **Borrowed slice**: `[]const u8` — does not own memory, do not free.
- **Owned slice**: obtained via `toOwnedSlice`, `alloc`, or `dupe` — caller must free.
- **Duplicate**: `const owned = try allocator.dupe(u8, borrowed);`

## Error Handling

No breaking changes to error unions or `try`/`catch` in 0.14 or 0.15. Syntax is stable:

```zig
fn mayFail() !u32 { return error.Oops; }

// try — propagate error to caller
const val = try mayFail();

// catch — handle inline
const val2 = mayFail() catch |err| blk: {
    std.log.err("failed: {}", .{err});
    break :blk 0;
};

// if with error union
if (mayFail()) |v| { _ = v; } else |err| { _ = err; }
```
