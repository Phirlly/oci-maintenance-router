# Inspect saved work

Invoke the deployed development Function with [list-notices.json](list-notices.json)
to find retained announcement IDs. Invocation permission is required. A successful
response has `status: "listed"` and `savedNotices.evidence: "RETAINED_IDS"`.
Use a returned ID in [review-notice.json](review-notice.json) to inspect it.

If `savedNotices.nextCursor` is present, repeat `list-notices` with that value as
`cursor`, even when `noticeIds` is empty. Each response returns at most 10 IDs.
On `listing_changed`, restart without a cursor. Listing is limited to 100 storage
pages per cursor chain; `listing_limit` requires operator investigation. It is
not a fixed inventory snapshot: new records behind your position require a restart.

A successful response has `status: "reviewed"` and `review.evidence: "LAST_SAVED"`.
It shows retained issue codes, revision/batch identifiers and resource indexes
(zero-based within the retained revision). Preparation evidence is marked
`LAST_BLOCKED`, `RECHECK_REQUIRED` or `HISTORICAL`; none establishes current health.

If `review.nextCursor` is present, repeat the request with the same `noticeId`
and that value as `cursor`. If `review_changed` is returned, restart without a
cursor. Read every page; an empty issue list is not a health or delivery check.

Inspection sends no alerts and performs no recovery or business-record writes.
It briefly acquires the worker lease; `coalesced` means another worker owns it.
Retry inspection later. Other unsuccessful results must not be treated as an
empty review or empty inventory. Listing validates saved identities, not current
issues; inspect each relevant ID separately. Recovery and automatic operator
notifications remain under development.
