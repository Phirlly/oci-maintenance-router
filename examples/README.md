# Inspect saved work

These instructions apply to the current development stack. For the published
v0.0.3 package, use its [deployment walkthrough](../docs/deployment-walkthrough.md).

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
issues; inspect each relevant ID separately. Automatic operator reporting is
implemented; this inspection does not establish its current health.

## Close saved work without sending

1. Invoke [review-closure.json](review-closure.json) with the saved notice ID. Read
   every `closureReview` page using its `nextCursor`. This lists retained revisions,
   batches and previous recovery outcomes.
2. Review one target by adding `kind: "CLOSE_REVISION"` and its `targetId`, or
   `kind: "CLOSE_BATCH"` and the batch ID. Start without a cursor. Read every page;
   the final `CLOSURE` item contains the code for an eligible target. On
   `review_changed`, restart. Missing or blocked targets have no usable code.
3. In the RM stack, select **Maintenance routing: Paused** and **Setup action: None**.
   Paste the code under **Advanced: close saved work**, enter a request number,
   and select **Close this saved work without sending**.
   Review Plan, then Apply. `closure_completed: true` confirms that exact request.

Closure is permanent. Closing a revision closes its unfinished work, including
unassigned resources; accepted deliveries and history stay intact. Earlier send
uncertainty remains visible. A `SENDING` batch blocks closure even after its saved
expiry. This inspection does not expire attempts or fetch current service data.

If Apply fails or its outcome is uncertain, inspect `closure_result` or run the
review again. Retry with the **same code and number**. For a rejected request or
changed intent, review again and use a new number. Number conflicts are enforced
within each notice; another notice has separate history. To remove configured
intent, clear the confirmation checkbox and **Apply**; Paused alone does not cancel
it. Clearing inputs or rolling back a package never reopens closed work.

Production acceptance for the development stack remains unfinished.
