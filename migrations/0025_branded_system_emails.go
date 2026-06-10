package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Restyle PocketBase's system emails (account verification, password reset,
// new-location login alert) with the same dark branded shell the notify
// emails use (internal/notify/templates/layout.html), so account mail no
// longer looks like a stock PocketBase install. The header logo is referenced
// as cid:mark — the system-mail hook (mailer.RegisterSystemMail) attaches the
// PNG whenever a body references it, mirroring how notify emails embed it.
//
// Only the bodies change; subjects keep PocketBase's defaults. {APP_NAME},
// {APP_URL}, {TOKEN} and {ALERT_INFO} are PB placeholders resolved at send.

// sysEmail wraps content in the branded shell, with an optional CTA button.
func sysEmail(content, ctaText, ctaURL string) string {
	cta := ""
	if ctaURL != "" {
		cta = `<table role="presentation" cellpadding="0" cellspacing="0" style="margin:26px 0 6px;"><tr><td bgcolor="#c8fb50" style="border-radius:999px;background:#c8fb50;">
<a href="` + ctaURL + `" target="_blank" rel="noopener" style="display:inline-block;padding:13px 30px;font-size:15px;font-weight:800;letter-spacing:.01em;color:#08110a;text-decoration:none;">` + ctaText + ` &rarr;</a>
</td></tr></table>`
	}
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="dark">
<meta name="supported-color-schemes" content="dark">
</head>
<body style="margin:0;padding:0;background:#0b0f1a;color:#f3f5fb;font-family:'Archivo',system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;-webkit-font-smoothing:antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" bgcolor="#0b0f1a" style="background:#0b0f1a;padding:28px 14px;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="width:100%;max-width:600px;background:#10131f;border:1px solid #283047;border-radius:18px;overflow:hidden;">

<tr><td height="5" style="height:5px;line-height:5px;font-size:0;background:#c8fb50;">&nbsp;</td></tr>

<tr><td style="padding:18px 28px;border-bottom:1px solid #1c2336;">
<table role="presentation" cellpadding="0" cellspacing="0"><tr>
<td style="padding-right:10px;vertical-align:middle;"><img src="cid:mark" width="20" height="35" alt="" style="display:block;border:0;height:35px;width:auto;"></td>
<td style="vertical-align:middle;font-size:18px;font-weight:800;letter-spacing:.04em;color:#f3f5fb;">WM&nbsp;<span style="color:#c8fb50;">Tips</span></td>
</tr></table>
</td></tr>

<tr><td style="padding:30px 28px 8px;">
` + content + cta + `
</td></tr>

<tr><td style="padding:20px 28px 24px;border-top:1px solid #1c2336;color:#6f7796;font-size:12px;line-height:1.6;">
<strong style="color:#8b93b0;">WM Tips</strong> — World Cup 2026 predictions with your friends.<br>
You're getting this about your WM Tips account. If you didn't request it, you can ignore this email.
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>`
}

var sysVerificationBody = sysEmail(`<div style="font-size:12px;font-weight:800;letter-spacing:.12em;text-transform:uppercase;color:#c8fb50;">Account</div>
<h1 style="margin:7px 0 14px;font-size:27px;line-height:1.12;font-weight:800;color:#f3f5fb;">Verify your email</h1>
<p style="margin:0;font-size:15px;line-height:1.6;color:#aeb6d0;">
Confirm this address to activate kickoff reminders, matchday recaps and league alerts for your {APP_NAME} account.
</p>`,
	"Verify email", "{APP_URL}/confirm-verification/{TOKEN}")

var sysResetPasswordBody = sysEmail(`<div style="font-size:12px;font-weight:800;letter-spacing:.12em;text-transform:uppercase;color:#c8fb50;">Account</div>
<h1 style="margin:7px 0 14px;font-size:27px;line-height:1.12;font-weight:800;color:#f3f5fb;">Reset your password</h1>
<p style="margin:0;font-size:15px;line-height:1.6;color:#aeb6d0;">
Click the button below to choose a new password for your {APP_NAME} account. The link is valid for a limited time.
</p>`,
	"Reset password", "{APP_URL}/confirm-password-reset/{TOKEN}")

var sysAuthAlertBody = sysEmail(`<div style="font-size:12px;font-weight:800;letter-spacing:.12em;text-transform:uppercase;color:#c8fb50;">Security</div>
<h1 style="margin:7px 0 14px;font-size:27px;line-height:1.12;font-weight:800;color:#f3f5fb;">Login from a new location</h1>
<p style="margin:0 0 20px;font-size:15px;line-height:1.6;color:#aeb6d0;">
We noticed a login to your {APP_NAME} account from a new location:
</p>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr>
<td bgcolor="#181d2e" style="background:#181d2e;border:1px solid #283047;border-radius:12px;padding:14px 18px;font-size:14px;line-height:1.6;color:#f3f5fb;">
{ALERT_INFO}
</td></tr></table>
<p style="margin:20px 0 0;font-size:15px;line-height:1.6;color:#aeb6d0;">
<strong style="color:#f3f5fb;">If this wasn't you</strong>, change your password right away to revoke access from all other locations. If this was you, you can disregard this email.
</p>`,
	"Open settings", "{APP_URL}/settings")

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.VerificationTemplate.Body = sysVerificationBody
		users.ResetPasswordTemplate.Body = sysResetPasswordBody
		users.AuthAlert.EmailTemplate.Body = sysAuthAlertBody
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		// Restore the PocketBase stock bodies, with the SPA links from
		// migrations 0008/0024 kept intact.
		users.VerificationTemplate.Body = `<p>Hello,</p>
<p>Thank you for joining us at {APP_NAME}.</p>
<p>Click on the button below to verify your email address.</p>
<p>
  <a class="btn" href="{APP_URL}/confirm-verification/{TOKEN}" target="_blank" rel="noopener">Verify</a>
</p>
<p>
  Thanks,<br/>
  {APP_NAME} team
</p>`
		users.ResetPasswordTemplate.Body = `<p>Hello,</p>
<p>Click on the button below to reset your password.</p>
<p>
  <a class="btn" href="{APP_URL}/confirm-password-reset/{TOKEN}" target="_blank" rel="noopener">Reset password</a>
</p>
<p><i>If you didn't ask to reset your password, you can ignore this email.</i></p>
<p>
  Thanks,<br/>
  {APP_NAME} team
</p>`
		users.AuthAlert.EmailTemplate.Body = `<p>Hello,</p>
<p>We noticed a login to your {APP_NAME} account from a new location:</p>
<p><em>{ALERT_INFO}</em></p>
<p><strong>If this wasn't you, you should immediately change your {APP_NAME} account password to revoke access from all other locations.</strong></p>
<p>If this was you, you may disregard this email.</p>
<p>
  Thanks,<br/>
  {APP_NAME} team
</p>`
		return app.Save(users)
	})
}
