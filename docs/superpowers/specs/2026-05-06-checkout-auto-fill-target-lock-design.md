# Checkout Auto-Fill Target Lock Design

## Goal

Make the `自动填地址` action write only into the checkout page that was most recently opened by this tool.

If the currently detected checkout page URL does not match that recorded URL, the action must fail without filling any fields.

## Scope

This change only affects the checkout address auto-fill flow.

It does not change:

- how payment links are generated
- how the incognito window is created
- how GoPay linking and payment are handled

## User-Facing Behavior

When the tool opens a payment link in the shared incognito window, it records that exact checkout URL as the latest managed checkout target.

When the user clicks `自动填地址`, the system:

1. checks that a latest managed checkout URL exists
2. finds the active checkout surface in the shared incognito browser
3. reads the detected page or frame URL
4. compares the detected URL with the recorded latest managed checkout URL
5. fills the form only if they match

If there is no recorded checkout URL, the tool returns a clear error.

If the recorded URL and detected URL do not match, the tool returns a clear error and does not write anything into the page.

## Recommended Approach

Use a strict URL lock with a single recorded value on the frontend and explicit verification on the backend before any field injection.

Why this approach:

- it matches the user's requirement exactly
- it avoids accidentally filling an older or unrelated Stripe page
- it keeps the implementation small by reusing the existing incognito and CDP pipeline

## Data Flow

### Frontend

- Add a dedicated variable such as `latestOpenedCheckoutURL`
- Update it only when `openPaymentInIncognito()` successfully opens a payment link
- Send that URL to `/api/checkout/auto-fill` as `expected_url`

### Backend

- Extend `handleCheckoutAutoFill` request body to accept `expected_url`
- Reject the request if `expected_url` is empty
- Discover the most relevant checkout page or Stripe frame through existing CDP logic
- Read its current URL before attempting any fill
- Compare detected URL against `expected_url`
- Only continue to fill fields if they match

## URL Matching Rule

Use strict normalized string equality after trimming whitespace.

Normalization should include:

- trim surrounding whitespace

Do not loosen matching to host-only or path-only matching in this change. A loose match would reintroduce the risk of filling the wrong page.

## Error Handling

Return distinct errors for:

- no recorded checkout URL
- CDP not ready
- no checkout page found
- detected checkout page URL mismatch
- checkout page found but no fillable form fields found

Suggested mismatch message:

`当前支付页与本工具最近一次打开的支付链接不一致，已拒绝自动填写。`

Suggested missing-target message:

`未找到本工具最近一次打开的支付链接页面，请重新打开支付链接后再试。`

## UI Response

On success:

- keep the existing success flow
- optionally mention that the fill was applied to the latest managed checkout page

On failure:

- show the backend error as the alert body
- do not claim partial success

## Implementation Notes

- Reuse existing `latestCheckoutURL` and `openPaymentInIncognito()` flow where possible
- Avoid storing this target in persistent browser storage unless already necessary; in-memory state is preferred for this guard
- Keep the field-filling logic unchanged after target verification passes

## Testing

Add focused tests for:

1. backend rejects empty `expected_url`
2. backend rejects URL mismatch
3. backend proceeds when detected URL equals expected URL
4. frontend sends the latest opened checkout URL to auto-fill

## Risks

- Stripe may render the editable address form inside a nested frame whose URL differs from the top-level payment page

Mitigation:

- compare against the actual fill target URL that the backend selects, not an unrelated page URL
- if needed, include both page URL and frame URL in diagnostics

## Success Criteria

- `自动填地址` only writes into the checkout page most recently opened by this tool
- any URL mismatch causes a hard failure with no field writes
- existing checkout fill behavior still works when the target matches
