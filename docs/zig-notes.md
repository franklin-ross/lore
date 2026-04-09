# Zig Notes

Notes on building with Zigß on macOS. The language has changed significantly from
earlier versions — many online examples are outdated.

## Strict Mutability

Variables that are never mutated must be `const`. Parameters that are
used cannot be discarded with `_ = param`. The compiler enforces both.

## Memory: Slices Into Freed Buffers

When you read a file with `readFileAlloc` and parse slices out of it,
those slices become dangling pointers when you free the file content.
You must `dupe` any strings you want to keep beyond the file buffer's
lifetime:

```zig
const content = try dir.readFileAlloc(allocator, path, max);
defer allocator.free(content);

// BAD: entity.name points into content, which gets freed
entity.name = header.name;

// GOOD: entity.name is an independent copy
entity.name = try allocator.dupe(u8, header.name);
```

This is the most common source of segfaults in the parser.

## Arena Allocator for Parsers

When building an in-memory data structure from parsed files, use an arena
allocator. All string data (entity names, descriptions, file content) goes
into the arena. Container metadata (ArrayList internal buffers) uses the
regular allocator. One `arena.deinit()` frees all string data at once.

```zig
var arena = std.heap.ArenaAllocator.init(gpa_allocator);
const arena_alloc = arena.allocator();

// String data — lives until arena.deinit()
const name = try arena_alloc.dupe(u8, raw_name);

// Container buffers — freed individually
var list: std.ArrayList(T) = .empty;
try list.append(gpa_allocator, item);
list.deinit(gpa_allocator);

// Free all string data at once
arena.deinit();
```

Be careful with the return value of `parseEntityHeader` or similar
functions — if you return slices that point into a stack-local buffer,
they become dangling pointers. Return slices into the input (which is
arena-owned) or embed fixed-size arrays in the return struct.

## Never Move On With Allocation Issues

Fix memory leaks immediately. The GPA in debug mode reports every leak
with a stack trace. Do not skip these or defer fixing them — they
compound quickly and become much harder to trace later. Treat a leak
report as a failing test.

## String Handling

Strings are `[]const u8`. No stdlib string type. Key patterns:

- **Borrowed slice**: `[]const u8` — does not own memory, do not free.
- **Owned slice**: obtained via `toOwnedSlice`, `alloc`, or `dupe` — caller must free.
- **Duplicate**: `const owned = try allocator.dupe(u8, borrowed);`

---

# Zig 0.15 API Reference

Quick reference for current (0.15.x) APIs. Covers the major breaking changes from 0.13/0.14.

## ArrayList (Now Unmanaged by Default)

`std.ArrayList(T)` no longer stores an allocator. Pass it to every method call.
The old `std.ArrayList(T).init(allocator)` pattern does not compile.

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
`deinit` and `getOrPut` take no allocator argument.

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

## File I/O (Writergate)

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

For simple output, skip the writer and use `File.writeAll` with `std.fmt.bufPrint`:

```zig
var buf: [4096]u8 = undefined;
const s = std.fmt.bufPrint(&buf, "x={d}\n", .{42}) catch return;
std.fs.File.stdout().writeAll(s) catch {};
```

## ArenaAllocator (Pinned — Must Not Be Moved)

ArenaAllocator is a **pinned** type in 0.15 — it holds internal pointers to its
own address, so copying or moving the struct (e.g. returning it by value from a
function) invalidates those pointers. This causes runtime panics
(`start index X is larger than end index 0`) or silent corruption / leaked pages.

```zig
var arena = std.heap.ArenaAllocator.init(std.heap.page_allocator);
defer arena.deinit();
const alloc = arena.allocator();

const buf = try alloc.alloc(u8, 256);
// No need to free individual allocations; arena.deinit() frees everything.
```

To store in a struct, initialise in-place (the return moves the whole struct, which is fine):

```zig
const MyThing = struct {
    arena: std.heap.ArenaAllocator,

    fn init(backing: std.mem.Allocator) MyThing {
        return .{ .arena = std.heap.ArenaAllocator.init(backing) };
    }
};
```

If the struct is heap-allocated or the arena needs to outlive the init scope, use a pointer:

```zig
// BAD: arena is moved when MyStruct is returned
fn make(gpa: Allocator) MyStruct {
    var arena = ArenaAllocator.init(gpa);
    return MyStruct{ .arena = arena };  // moved!
}

// GOOD: arena is heap-allocated, pointer is stable
fn make(gpa: Allocator) !MyStruct {
    const arena = try gpa.create(ArenaAllocator);
    arena.* = ArenaAllocator.init(gpa);
    return MyStruct{ .arena = arena };  // pointer, not moved
}

// deinit:
self.arena.deinit();
self.allocator.destroy(self.arena);
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

Tests run with `zig build test`. Zig runs tests from both the library module and
the executable module separately.

## Testing

`std.testing.allocator` detects leaks. `std.testing.expect` is the primary assertion.
Tests are inline `test` blocks in source files.

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

## Child Process

Use `Child.init(argv, allocator)` then set fields. `collectOutput` reads
stdout/stderr into ArrayLists.

```zig
var child = std.process.Child.init(argv, allocator);
child.cwd = "/some/dir";
child.stdout_behavior = .Pipe;  // Note: capitalised in 0.15
child.stderr_behavior = .Pipe;

try child.spawn();

var stdout_buf: std.ArrayList(u8) = .empty;
var stderr_buf: std.ArrayList(u8) = .empty;
try child.collectOutput(allocator, &stdout_buf, &stderr_buf, 1024 * 1024);
const term = try child.wait();
```

## Temp Directories in Tests

```zig
var tmp = std.testing.tmpDir(.{});
defer tmp.cleanup();

var f = try tmp.dir.createFile("test.txt", .{});
try f.writeAll("content");
f.close();

const root = try tmp.dir.realpathAlloc(allocator, ".");
defer allocator.free(root);
```

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
