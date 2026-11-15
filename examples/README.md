# Inspect saved work

The development package accepts [review-notice.json](review-notice.json) as a
Function invocation body. Replace the example with a known announcement OCID.
This requires permission to invoke the deployed router Function.

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
empty review. This command does not list retained announcement IDs; complete
operator discovery and recovery controls remain under development.
