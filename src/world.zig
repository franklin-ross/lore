const std = @import("std");
const entity = @import("entity.zig");

const Entity = entity.Entity;
const Description = entity.Description;
const Reference = entity.Reference;
const SearchResult = entity.SearchResult;
const Issue = entity.Issue;

pub const World = struct {
    /// Arena owns all string data allocated during parsing. Heap-allocated
    /// to avoid invalidation when World is moved/returned.
    arena: *std.heap.ArenaAllocator,
    allocator: std.mem.Allocator,
    entities: std.ArrayList(Entity),
    references: std.StringHashMap(std.ArrayList(Reference)),

    pub fn deinit(self: *World) void {
        for (self.entities.items) |*e| {
            e.aliases.deinit(self.allocator);
            e.descriptions.deinit(self.allocator);
        }
        self.entities.deinit(self.allocator);
        var it = self.references.iterator();
        while (it.next()) |entry| {
            entry.value_ptr.deinit(self.allocator);
        }
        self.references.deinit();
        self.arena.deinit();
        self.allocator.destroy(self.arena);
    }

    const max_ambiguous = 16;

    pub const FindResult = union(enum) {
        found: *const Entity,
        not_found,
        ambiguous: struct {
            entities: [max_ambiguous]*const Entity,
            count: usize,

            pub fn items(self: *const @This()) []const *const Entity {
                return self.entities[0..self.count];
            }
        },

        /// Returns the entity if found, null otherwise.
        pub fn get(self: FindResult) ?*const Entity {
            return switch (self) {
                .found => |e| e,
                else => null,
            };
        }
    };

    pub fn findEntity(self: *const World, name: []const u8) FindResult {
        // Support disambiguation syntax: "Barovia (town)"
        if (entity.parseDisambiguation(name)) |disambig| {
            for (self.entities.items) |*e| {
                if (e.entity_type) |et| {
                    if (!std.ascii.eqlIgnoreCase(et, disambig.entity_type)) continue;
                    if (std.ascii.eqlIgnoreCase(e.name, disambig.name)) return .{ .found = e };
                    if (entity.nameMatchesAlias(e, disambig.name)) return .{ .found = e };
                }
            }
            return .not_found;
        }

        // Collect all matches to detect ambiguity
        var matches: [max_ambiguous]*const Entity = undefined;
        var match_count: usize = 0;

        for (self.entities.items) |*e| {
            if (std.ascii.eqlIgnoreCase(e.name, name) or entity.nameMatchesAlias(e, name)) {
                if (match_count < matches.len) {
                    matches[match_count] = e;
                    match_count += 1;
                }
            }
        }

        return switch (match_count) {
            0 => .not_found,
            1 => .{ .found = matches[0] },
            else => .{ .ambiguous = .{ .entities = matches, .count = match_count } },
        };
    }

    pub fn getReferences(self: *const World, name: []const u8) std.ArrayList(Reference) {
        if (self.references.get(name)) |refs| return refs;
        var it = self.references.iterator();
        while (it.next()) |entry| {
            if (std.ascii.eqlIgnoreCase(entry.key_ptr.*, name)) return entry.value_ptr.*;
        }
        var empty: std.ArrayList(Reference) = .empty;
        _ = &empty;
        return empty;
    }

    pub fn search(self: *const World, query: []const u8) std.ArrayList(SearchResult) {
        var results: std.ArrayList(SearchResult) = .empty;
        for (self.entities.items) |e| {
            for (e.descriptions.items) |desc| {
                if (entity.containsIgnoreCase(desc.text, query)) {
                    results.append(self.allocator, .{
                        .file = desc.file,
                        .line = desc.line,
                        .context = desc.text,
                    }) catch {};
                }
            }
        }
        return results;
    }

    pub fn check(self: *const World) std.ArrayList(Issue) {
        _ = self;
        var empty: std.ArrayList(Issue) = .empty;
        _ = &empty;
        return empty;
    }
};
