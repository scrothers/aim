// Package notify posts a compact card to Discord or Slack incoming webhooks
// for each new message addressed to the operator. Webhooks are stored in the
// settings (see mail.NotifySettings); their URLs are secrets and leave this
// package only masked.
//
// Delivery is asynchronous and per webhook: at most one post per coalescing
// window (bursts become one summary card), a bounded queue, retries with
// backoff, and 429 retry_after is honored.
package notify
