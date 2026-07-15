package service

import "fmt"

// SessionConfirmationEmail is sent once, at session creation, when
// send_confirmation_email is true on the input.
func SessionConfirmationEmail(sessionName, batchNumber, date, startTime, joinLink string) (subject, html string) {
	subject = fmt.Sprintf("Session scheduled: %s", sessionName)
	html = fmt.Sprintf(`
		<p>Hi,</p>
		<p>A new session <strong>%s</strong> has been scheduled for batch <strong>%s</strong>.</p>
		<p><strong>Date:</strong> %s<br><strong>Time:</strong> %s</p>
		%s
		<p>— Vedex</p>`,
		sessionName, batchNumber, date, startTime, joinLinkHTML(joinLink))
	return
}

// SessionReminderEmail is sent by the reminder scheduler the moment a
// session's scheduled start time arrives.
func SessionReminderEmail(sessionName, batchNumber, joinLink string) (subject, html string) {
	subject = fmt.Sprintf("Starting now: %s", sessionName)
	html = fmt.Sprintf(`
		<p>Your session <strong>%s</strong> for batch <strong>%s</strong> is starting now.</p>
		%s
		<p>— Vedex</p>`,
		sessionName, batchNumber, joinLinkHTML(joinLink))
	return
}

// BatchStartReminderEmail is sent by the batch reminder scheduler the day
// before a batch's start date.
func BatchStartReminderEmail(batchNumber, startDate string) (subject, html string) {
	subject = fmt.Sprintf("Your batch %s starts tomorrow", batchNumber)
	html = fmt.Sprintf(`
		<p>Hi,</p>
		<p>This is a reminder that your batch <strong>%s</strong> starts on <strong>%s</strong>.</p>
		<p>— Vedex</p>`,
		batchNumber, startDate)
	return
}

// BatchCreatedEmail is sent to the batch manager(s) and any students enrolled
// at creation time when a new batch is created.
func BatchCreatedEmail(batchNumber, courseName, startDate, endDate string) (subject, html string) {
	subject = fmt.Sprintf("New batch created: %s", batchNumber)
	html = fmt.Sprintf(`
		<p>Hi,</p>
		<p>A new batch <strong>%s</strong> (%s) has been created.</p>
		<p><strong>Start date:</strong> %s<br><strong>End date:</strong> %s</p>
		<p>— Vedex</p>`,
		batchNumber, courseName, startDate, endDate)
	return
}

// CommunityCreatedEmail is sent to mentors and team leads when a new
// community is created.
func CommunityCreatedEmail(name, batchNumber string) (subject, html string) {
	subject = fmt.Sprintf("New community: %s", name)
	html = fmt.Sprintf(`
		<p>Hi,</p>
		<p>A new community <strong>%s</strong> has been created for batch <strong>%s</strong>.</p>
		<p>— Vedex</p>`,
		name, batchNumber)
	return
}

// CommunityMemberAddedEmail is sent to a user when they're added to a community.
func CommunityMemberAddedEmail(name string) (subject, html string) {
	subject = fmt.Sprintf("Added to community: %s", name)
	html = fmt.Sprintf(`
		<p>Hi,</p>
		<p>You've been added to the community <strong>%s</strong>.</p>
		<p>— Vedex</p>`,
		name)
	return
}

// ModuleCreatedEmail is sent broadly (students, mentors, team leads) when a
// new module is added, mirroring the existing broadcast in-app notification.
func ModuleCreatedEmail(moduleName, moduleBranch string) (subject, html string) {
	subject = fmt.Sprintf("New module: %s", moduleName)
	html = fmt.Sprintf(`
		<p>Hi,</p>
		<p>A new module <strong>%s</strong> (%s) has been added.</p>
		<p>— Vedex</p>`,
		moduleName, moduleBranch)
	return
}

// ForgotPasswordOTPEmail is sent when a user requests a password reset. The
// code is valid for 5 minutes.
func ForgotPasswordOTPEmail(otp string) (subject, html string) {
	subject = "Your Vedex password reset code"
	html = fmt.Sprintf(`
		<p>Hi,</p>
		<p>Use the code below to reset your password. It expires in <strong>5 minutes</strong>.</p>
		<p style="font-size:24px; font-weight:bold; letter-spacing:4px;">%s</p>
		<p>If you didn't request this, you can safely ignore this email.</p>
		<p>— Vedex</p>`,
		otp)
	return
}

// StaffWelcomeEmail is sent when an admin creates a new staff or student
// account, carrying the one-time-shown temporary password.
func StaffWelcomeEmail(firstName, roleLabel, email, password, loginURL string) (subject, html string) {
	subject = "Your Vedex account has been created"
	html = fmt.Sprintf(`
		<p>Hi %s,</p>
		<p>An account has been created for you on Vedex as a <strong>%s</strong>.</p>
		<p><strong>Email:</strong> %s<br><strong>Temporary password:</strong> %s</p>
		<p>Please log in and change your password as soon as possible.</p>
		<p><a href="%s">Log in to Vedex</a></p>
		<p>— Vedex</p>`,
		firstName, roleLabel, email, password, loginURL)
	return
}

// LeaveDecisionEmail is sent when an admin approves or rejects a leave request.
func LeaveDecisionEmail(firstName, status, fromDate, toDate, adminNote string) (subject, html string) {
	verb := "approved"
	if status == "rejected" {
		verb = "rejected"
	}
	subject = fmt.Sprintf("Your leave request has been %s", verb)
	noteHTML := ""
	if adminNote != "" {
		noteHTML = fmt.Sprintf(`<p><strong>Note:</strong> %s</p>`, adminNote)
	}
	html = fmt.Sprintf(`
		<p>Hi %s,</p>
		<p>Your leave request for <strong>%s to %s</strong> has been <strong>%s</strong>.</p>
		%s
		<p>— Vedex</p>`,
		firstName, fromDate, toDate, verb, noteHTML)
	return
}

func joinLinkHTML(link string) string {
	if link == "" {
		return ""
	}
	return fmt.Sprintf(`<p><a href="%s">Join session</a></p>`, link)
}
