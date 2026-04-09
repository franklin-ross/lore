# Zig 0.15 Notes

Notes on building with Zig 0.15.2 on macOS. The language has changed
significantly from earlier versions — many online examples are outdated.

## ArrayList Is Now Unmanaged

`std.ArrayList(T)` no longer stores an allocator. You pass it to every
operation.

```zig
// Init
var list: std.ArrayList(u8) = .empty;

// Append
try list.append(allocator, value);

// Deinit — needs allocator AND must be var (not const)
list.deinit(allocator);

// toOwnedSlice also needs allocator
const slice = try list.toOwnedSlice(allocator);
```

The old `std.ArrayList(T).init(allocator)` pattern does not compile.

## HashMap Is Still Managed

`std.StringHashMap` and `std.HashMap` still store the allocator from
`init`. But `deinit` takes no arguments:

```zig
var map = std.StringHashMap(V).init(allocator);
map.deinit();  // no allocator arg

const result = try map.getOrPut(key);  // no allocator arg
```

## File I/O Changed

`std.io.getStdOut()` no longer exists. Use:

```zig
const stdout = std.fs.File.stdout();
stdout.writeAll("hello\n") catch {};
```

The `File.writer()` method now requires a buffer parameter and returns a
`Writer` struct with an `interface` field. For simple output, skip the
writer and use `File.writeAll` with `std.fmt.bufPrint`:

```zig
var buf: [4096]u8 = undefined;
const s = std.fmt.bufPrint(&buf, "x={d}\n", .{42}) catch return;
std.fs.File.stdout().writeAll(s) catch {};
```

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

## Build System

`zig init` generates a clean project with library + executable split.
`build.zig` uses a DSL — `b.addModule` for libraries, `b.addExecutable`
for binaries, `b.addTest` for test runners. Tests run with `zig build test`.

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

## ArenaAllocator Must Not Be Moved

`ArenaAllocator` has internal state that becomes invalid if the struct
is moved (e.g. returned by value from a function). Heap-allocate it
if it needs to live inside a struct that gets returned:

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

This cost us time debugging — the GPA reported leaked arena pages even
though `arena.deinit()` was called, because the move invalidated the
arena's internal page list.

## Never Move On With Allocation Issues

Fix memory leaks immediately. The GPA in debug mode reports every leak
with a stack trace. Do not skip these or defer fixing them — they
compound quickly and become much harder to trace later. Treat a leak
report as a failing test.

## Testing

Tests are inline `test` blocks in source files. Use `std.testing.allocator`
which detects memory leaks. Zig runs tests from both the library module and
the executable module separately.
