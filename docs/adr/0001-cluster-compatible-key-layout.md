# 0001: Cluster-compatible Redis key layout

Status: accepted. Decided 2026-10-01 in [issue #9](https://github.com/danielcaze/distributed-rate-limiter/issues/9).

## Context

Each admission algorithm runs as one Lua script so that a check is atomic. Redis expects a script to receive every key it touches through `KEYS`. Redis Cluster depends on this: it splits keys across nodes by hash slot and routes a script to the node that owns the slots of its declared keys. A script that touches an undeclared key, or keys in different slots, cannot run on a cluster.

The first fixed window design put the window start in the key name, `rl:fw:<key>:<window_start>`, so that a new window was a new key and the script only needed `INCR`. The window start comes from `now`, and `now` comes from Redis `TIME` inside the script, which keeps a single clock for every service instance. Go therefore cannot know the key name before the call and cannot declare it. The sliding window counter had the same problem with two time-named keys.

## Decision

Key names never depend on time, and every script declares all its keys in `KEYS`.

- Fixed window: one key `rl:fw:<key>` holding a hash `{ ws, count }`. The script compares the stored window start with the current one, increments in the same window, and writes the new window start with `count = 1` in a new window.
- Sliding window counter: one key `rl:swc:<key>` holding a hash `{ ws, prev, cur }`, rotated by the script when the window changes.
- Sliding window log: two keys, `rl:swl:{<key>}` and `rl:swl:{<key>}:seq`. The hash tag `{<key>}` makes Redis Cluster hash only the client key, so both land in the same slot. Tagging by client key still spreads clients across nodes.
- Token bucket: one key `rl:tb:<key>`, unchanged apart from the prefix.

## Alternatives

| Option | Why not |
| --- | --- |
| The script builds the time-named key | Simplest script, but it touches an undeclared key and cannot run on a cluster. |
| Go computes `now` and passes the full key | Declarable, but it gives up the single Redis clock. Instances with clock skew would disagree on which window a call near a boundary belongs to. |
| A fixed key reset only by its TTL | Redis expiry follows Redis's real clock, not the test time passed through `checkAt`, so tests could not control window changes. TTL also stays cleanup only ([M2 contract](../m2-contract.md#state-layout-and-lifetime)). |

## Consequences

- The fixed window and counter scripts gain a read and a branch that compares the window start. Both stay inside the same `EVALSHA`, so there is no extra network round trip. One more command inside Redis costs microseconds against a round trip of hundreds.
- Memory per key changes slightly: a small hash instead of a plain integer, with a shorter key name. The M2 benchmark measures it.
- The layout works on a single Redis today and on a Redis Cluster later without changing keys or scripts. M2 does not test on a cluster, so that claim is untested.
- Revisit if the project drops the single Redis clock, or if a cluster test shows another constraint.
