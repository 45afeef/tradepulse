# Alerts

An alert interrupts you only for what matters. You say what to watch for; when a new item matches, you are told right away with the reason.

## Creating an alert

Select **New alert** and choose a template or **Custom**. Then:

1. **Name** it so you recognise it in your history.
2. **Alert me about** a topic, a keyword or a source, and add one or more values. Any of them matching is enough.
3. Optionally open **advanced filters**: limit it to certain event types and add conditions, for example "price change is at least 2". All conditions must hold.
4. Choose how often it may fire, how important it is and where it should be delivered.
5. Check the **preview**, then save. You can have up to 50 alerts.

## Reading the preview

The preview replays the items the server has recently seen against your draft and tells you how many alerts it would have produced in the last day, with examples. It is the fastest way to avoid noisy alerts. Right after the server restarts it has seen less, so the numbers are lower until more items arrive.

## Cooldown and importance

- **At most one alert every** is the cooldown. After an alert fires, the rule stays quiet for that long even if more items match. The default is 5 minutes; longer means fewer interruptions.
- **Importance** (info, notable, high) is shown on the alert and in your history so you can tell what matters. It does not change when the alert fires.

## Where alerts go

Alerts always appear in the app, live, as a notification on screen. You can also send them to your own systems with a [webhook](/help/delivery).

## Why an alert gets paused {#why-an-alert-gets-paused}

To protect you from floods, a rule that fires more than 30 alerts in an hour (the default; your operator can change it) is **paused automatically**, and you see a notice. The rule is not deleted. Tighten it with a keyword, a condition or a longer cooldown, then select **Resume**.

## History and ratings

The **History** tab lists every alert that fired, newest first, with the reason. Mark each one **Useful** or **Not useful**. This is how alert quality is measured and improved.

## Turning alerts off

Use the switch on a rule to pause it, or delete it. Past alerts stay in your history for 30 days.
