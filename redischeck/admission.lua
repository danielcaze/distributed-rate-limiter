-- Pending admission implementation. This file intentionally contains no commands.
-- Input contract for the later script: KEYS[1] is the bucket key, ARGV[1] is
-- the shared capacity (positive whole tokens), and ARGV[2] is the shared refill
-- rate (positive tokens per second). Production time comes from Redis TIME;
-- deterministic time override is available only through an internal test path.
-- Output contract: a four-element array containing allowed (0 or 1), remaining
-- (whole admissions), reset_at (Unix microseconds in UTC), and retry_after
-- (nonnegative microseconds). reset_at is when the bucket would next be full
-- with no more requests; retry_after is zero on admission and otherwise the
-- earliest next admission. Admission is atomic per key. An allowed decision
-- consumes one admission, and a denial consumes none.
local MICROS_PER_SECOND = 1000000

local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refillRate = tonumber(ARGV[2])
local time = redis.call('TIME')
local timeInUnix = time[1]
local timeMicrosseconds = time[2]
local now = tonumber(timeInUnix) * MICROS_PER_SECOND + tonumber(timeMicrosseconds)

local res = redis.call('HMGET', key, 'tokens', 'ts')

local isNew = res[1] == false and res[2] == false

local ts, tokens
if isNew then
  ts = 0
  tokens = capacity
else
  ts = tonumber(res[2])
  tokens = tonumber(res[1])
end

tokens = math.min(capacity, tokens + (now - ts) / MICROS_PER_SECOND * refillRate)

local allowed = tokens >= 1 and 1 or 0

-- state persists only when a token is consumed; a denied request does not alter the bucket.
if allowed == 1 then
  tokens = tokens - 1
  redis.call('HMSET', key, 'tokens', tokens, 'ts', now)
end

local remaining = math.floor(tokens)
local resetAt = now + math.ceil((capacity - tokens) / refillRate * MICROS_PER_SECOND)
local retryAfter = allowed == 1 and 0 or math.ceil((1 - tokens) / refillRate * MICROS_PER_SECOND)

local response = { allowed, remaining, resetAt, retryAfter }

return response
