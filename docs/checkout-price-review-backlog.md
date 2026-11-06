# Checkout price review backlog

The website checkout summary currently displays the saved cart goods total and labels delivery as free, while order submission can calculate and add supplier delivery fees. The website also has no explicit review and confirmation when prices change between display and submission. A buyer can therefore see a different total from the one used when the order is placed.

This item is deferred for an owner decision; Batch 1A must not change the website checkout flow.

Options to consider:

1. Show a server-calculated itemized total, including each supplier's delivery fee, on checkout and require a new confirmation whenever any amount changes.
2. Show an updated itemized review only when an amount changes, retaining the current checkout screen otherwise.

Either option needs Arabic and English messages, preservation of the cart, protection against another change between review and submission, and tests proving unchanged orders proceed normally. Confirm how standalone offers, negotiations, taxes, and delivery quotes should appear before implementation.
