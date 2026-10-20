# Inject a missed internal update in the demo

The MVP creates a genuine PayPal Sandbox payout but deliberately skips the corresponding internal payout record update. This makes the primary incident repeatable without depending on live webhook delivery, which remains the first feature after P0. The interface must label the skipped update as an injected fault and keep genuine PayPal responses distinguishable from it.
