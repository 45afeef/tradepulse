# Operator handbook

For the people who run a TradePulse workspace: add sources, invite members, keep things healthy and watch quality. You see the **Operator** section in the sidebar (or the shield icon on a phone).

## Your first hour

The **Overview** page shows a checklist until the workspace is running:

1. **Add your first source.** Start with an RSS feed from a site your members care about. See [Choosing and tuning sources](/help/sources-guide).
2. **Test it, then start it.** The wizard shows what would be read before anything is saved.
3. **Watch items flow in.** The source page shows runs, items collected and the last error.
4. **Invite a few members.** Start with five to ten people who will give honest feedback.

## Inviting and growing

Under **Invites** create a code, optionally tied to an email address and with a note only you see. Share the **signup link**; it already contains the code. Codes work once and expire (the form suggests 7 days). The tabs show who is pending, who accepted and what expired. Revoke a code at any time.

Grow in small steps: invite, watch what members follow and rate, adjust sources, then invite more.

## Members

**Members** lists everyone. You can **suspend** (signs them out, stops alerts), **reactivate**, or **reset a password**: a temporary password is shown once; hand it over securely and ask them to change it under Settings. You cannot act on your own account.

## Publishing announcements

**Publish** posts a message through a manual source. It goes through matching like any item, so members who follow the source or topics receive it and alerts can fire. Create and start a manual source first.

## Keeping quality high

- **Alert precision** on the Overview is the share of rated alerts members marked useful. The target is 70%. It needs at least 10 ratings to show.
- A banner lists sources or alert rules that members mostly rate **Not useful** (at least three ratings and under half useful). Tighten or retire them.
- Ask members to use 👍/👎; it is the only signal that tells you what works.

## Source health

| Health | Meaning |
|---|---|
| ok | Working and on schedule. |
| pending | Created but not run yet. |
| degraded | Recent failures, still trying. |
| failing | Failing repeatedly; the last error is shown. |
| overdue | Should have run but did not; ask your technical owner. |
| disabled | Stopped by you. |

Open a source to see its recent runs and the last error, run it now, change its schedule or topics, or stop it.

## Audit log

**Audit log** records every operator action with who did it, filterable and exportable as CSV. Entries are kept for one year. Overriding or restoring a source's `robots.txt` handling is recorded here with the reason you gave.

## A simple weekly routine

1. Check the Overview for attention banners.
2. Review noisy sources and rules.
3. Look at pending invites and follow up.
4. Skim the audit log for anything unexpected.

Infrastructure topics (deployments, backups, secrets) belong to your technical owner and are covered in the project's operations runbook.
