# India preview offer

- One preview per UTC month for a guest, three total for a signed-in account. A current guest reservation follows its review when claimed. Reopening and unlocking a saved report use no preview. Existing usage stays counted when the limits change. Guest identity follows the existing browser identifier.
- The server default for new INR orders is ₹169. If production already sets `PRICE_INR_PAISE`, set it to `16900` in the existing deployment configuration. Check the public payment configuration after deployment. Existing recoverable orders keep the price already quoted. The USD price follows its current configuration.
- Deploy the API first and then the UI. Verify a fresh guest preview, two remaining after claim, price visibility before upload, and checkout from the saved preview. Existing database tables are sufficient.
- The free preview still generates and saves the detailed review, so count AI usage from both purchased and unpurchased reports. Establish budget enforcement before sending ad traffic.

## First launch cohort

Use completed job match previews from a fixed launch cohort as the denominator. Join confirmed INR purchases through `report_unlocks` and `payments` by analysis ID, count distinct unlocked reports, and exclude refunded payments for net conversion. Give each preview the same conversion window. A repeat buyer has paid for at least two different reviews. Compare ad spend and all AI usage for the cohort to the number of confirmed purchases. Keep cost coverage and unknown usage visible when using the AI usage reports. This release does not add ad attribution or new browser events for opens and meaningful actions.
