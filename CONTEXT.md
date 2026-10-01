# Rate limiting

Shared language for requests governed by a token bucket policy.

## Language

**Bucket**:
The quota state for one key under the shared policy. Each new key starts with its own full bucket.

**Capacity**:
The maximum number of tokens a bucket can hold.

**Admission**:
The decision on whether a request may consume one token from its bucket.

**Remaining**:
The number of tokens available in a bucket after an admission decision.

**Refill**:
The continuous restoration of tokens in a bucket over time, up to capacity.

**Limit**:
The number of admissions the shared policy allows per period. For the token bucket it is the capacity.

**Period**:
The time over which the limit applies. For window algorithms it is the window length; for the token bucket it is the time an empty bucket takes to fill.
