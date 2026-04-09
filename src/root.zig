pub const config = @import("config.zig");
pub const entity = @import("entity.zig");
pub const parser = @import("parser.zig");
pub const refs = @import("refs.zig");
pub const world = @import("world.zig");

test {
    _ = @import("config.zig");
    _ = @import("entity.zig");
    _ = @import("parser.zig");
    _ = @import("refs.zig");
    _ = @import("world.zig");
    _ = @import("lore_test.zig");
}
