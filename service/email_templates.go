package service

import "fmt"

// emailSignatureHTML is appended to the end of every outgoing email so the
// sign-off and support contact stay identical across templates.
const emailSignatureHTML = `
	<p>Warm regards,<br>
	<strong>The Vedxlence Support Team</strong></p>
	<hr style="border:none;border-top:1px solid #e5e5e5;margin:20px 0;">
	<p style="font-size:12px;color:#888888;">
		This is an automated message from Vedxlence. For any questions, reach out to us at
		<a href="mailto:support@vedxlence.in">support@vedxlence.in</a>.
	</p>`

// SessionConfirmationEmail is sent once, at session creation, when
// send_confirmation_email is true on the input.
func SessionConfirmationEmail(sessionName, batchNumber, date, startTime, joinLink string) (subject, html string) {
	subject = fmt.Sprintf("Session Scheduled – %s", sessionName)
	html = fmt.Sprintf(`
		<p>Hello,</p>
		<p>A new session has been scheduled for your batch in Vedxlence. Please find the session details below:</p>
		<p><strong>Session Name:</strong> %s<br>
		<strong>Batch Number:</strong> %s<br>
		<strong>Date:</strong> %s<br>
		<strong>Time:</strong> %s</p>
		%s
		<p>Please ensure that you join the session on time and verify your internet connection and device setup beforehand to ensure a smooth experience.</p>
		<p>If you have any questions or require assistance, please contact your instructor or the support team.</p>
		<p>We look forward to your participation.</p>`,
		sessionName, batchNumber, date, startTime, joinLinkHTML(joinLink)) + emailSignatureHTML
	return
}

// SessionReminderEmail is sent by the reminder scheduler the moment a
// session's scheduled start time arrives.
func SessionReminderEmail(sessionName, batchNumber, joinLink string) (subject, html string) {
	subject = fmt.Sprintf("Reminder: Your Session Is Starting Now – %s", sessionName)
	html = fmt.Sprintf(`
		<p>Hello,</p>
		<p>This is a reminder that your scheduled session is starting now.</p>
		<p>Please find the session details below:</p>
		<p><strong>Session Name:</strong> %s<br>
		<strong>Batch Number:</strong> %s</p>
		%s
		<p>Please click the join link above to access the session. We recommend joining a few minutes early to ensure your audio, video, and internet connection are working properly.</p>
		<p>If you experience any issues joining the session, please contact your instructor or the support team for assistance.</p>
		<p>We look forward to your participation.</p>`,
		sessionName, batchNumber, joinLinkHTML(joinLink)) + emailSignatureHTML
	return
}

// BatchStartReminderEmail is sent by the batch reminder scheduler the day
// before a batch's start date.
func BatchStartReminderEmail(batchNumber, startDate string) (subject, html string) {
	subject = "Reminder: Your Batch Begins Tomorrow"
	html = fmt.Sprintf(`
		<p>Hello,</p>
		<p>This is a friendly reminder that your batch is scheduled to begin <strong>tomorrow</strong>.</p>
		<p>Please find the details below:</p>
		<p><strong>Batch Number:</strong> %s<br>
		<strong>Start Date:</strong> %s</p>
		<p>We wish you a successful learning journey.</p>`,
		batchNumber, startDate) + emailSignatureHTML
	return
}

// BatchCreatedEmail is sent to the batch manager(s) and any students enrolled
// at creation time when a new batch is created.
func BatchCreatedEmail(batchNumber, courseName, startDate, endDate string) (subject, html string) {
	subject = fmt.Sprintf("New Batch Created – %s", batchNumber)
	html = fmt.Sprintf(`
		<p>Hello,</p>
		<p>We are pleased to inform you that a new batch has been created in Vedxlence.</p>
		<p>The batch details are as follows:</p>
		<p><strong>Batch Number:</strong> %s<br>
		<strong>Course:</strong> %s<br>
		<strong>Start Date:</strong> %s<br>
		<strong>End Date:</strong> %s</p>
		<p>Please review the batch information.</p>
		<p>If you have any questions or require additional assistance, please contact your administrator.</p>`,
		batchNumber, courseName, startDate, endDate) + emailSignatureHTML
	return
}

// CommunityCreatedEmail is sent to mentors and team leads when a new
// community is created.
func CommunityCreatedEmail(name, batchNumber string) (subject, html string) {
	subject = fmt.Sprintf("New Community Created – %s", name)
	html = fmt.Sprintf(`
		<p>Hello,</p>
		<p>We are pleased to inform you that a new community has been created in Vedxlence for your batch.</p>
		<p>Please find the community details below:</p>
		<p><strong>Community Name:</strong> %s<br>
		<strong>Batch Number:</strong> %s</p>
		<p>This community serves as a dedicated space for announcements, discussions, collaboration, and knowledge sharing among participants. We encourage you to join the community and actively engage with fellow members throughout your learning journey.</p>
		<p>If you have any questions or require assistance accessing the community, please contact support team.</p>
		<p>We look forward to your active participation.</p>`,
		name, batchNumber) + emailSignatureHTML
	return
}

// CommunityMemberAddedEmail is sent to a user when they're added to a community.
func CommunityMemberAddedEmail(name string) (subject, html string) {
	subject = fmt.Sprintf("You Have Been Added to a Community – %s", name)
	html = fmt.Sprintf(`
		<p>Hello,</p>
		<p>We are pleased to inform you that you have been added as a member of the following community in Vedxlence:</p>
		<p><strong>Community Name:</strong> %s</p>
		<p>As a member of this community, you will be able to participate in discussions, receive important announcements, collaborate with other members, and stay updated on activities related to your batch or course.</p>
		<p>We encourage you to actively engage with the community and contribute to meaningful discussions.</p>
		<p>If you have any questions or experience any issues accessing the community, please contact support team.</p>
		<p>We look forward to your participation.</p>`,
		name) + emailSignatureHTML
	return
}

// ModuleCreatedEmail is sent broadly (students, mentors, team leads) when a
// new module is added, mirroring the existing broadcast in-app notification.
func ModuleCreatedEmail(moduleName, moduleBranch string) (subject, html string) {
	subject = fmt.Sprintf("New module: %s", moduleName)
	html = fmt.Sprintf(`
		<p>Hello,</p>
		<p>A new module <strong>%s</strong> (%s) has been added.</p>`,
		moduleName, moduleBranch) + emailSignatureHTML
	return
}

// ForgotPasswordOTPEmail is sent when a user requests a password reset. The
// code is valid for 5 minutes.
func ForgotPasswordOTPEmail(otp string) (subject, html string) {
	subject = "Password Reset Verification Code"
	html = fmt.Sprintf(`
		<p>Hello,</p>
		<p>We received a request to reset the password for your Vedxlence account.</p>
		<p>Please use the verification code below to continue with the password reset process:</p>
		<p style="font-size:24px; font-weight:bold; letter-spacing:4px;">%s</p>
		<p>This code is valid for <strong>5 minutes</strong>. For your security, do not share this code with anyone.</p>`,
		otp) + emailSignatureHTML
	return
}

// LoginOTPEmail is sent on every password login as a mandatory second
// factor. The code is valid for 5 minutes and single-use.
func LoginOTPEmail(otp string) (subject, html string) {
	subject = "Your Vedxlence Login Verification Code"
	html = fmt.Sprintf(`
		<p>Hello,</p>
		<p>To complete your login-in to your Vedxlence account, please use the verification code below:</p>
		<p style="font-size:24px; font-weight:bold; letter-spacing:4px;">%s</p>
		<p>This code is valid for <strong>5 minutes</strong> and can only be used once.</p>
		<p>For your security, please do not share this code with anyone, including the Vedxlence support team. Vedxlence will never ask you to disclose your verification code.</p>`,
		otp) + emailSignatureHTML
	return
}

// StaffWelcomeEmail is sent when an admin creates a new staff or student
// account, carrying the one-time-shown temporary password.
func StaffWelcomeEmail(firstName, roleLabel, email, password, loginURL string) (subject, html string) {
	subject = "Welcome to Vedxlence – Your Account Has Been Created"
	html = fmt.Sprintf(`
		<p>Hello %s,</p>
		<p>Welcome to Vedxlence!</p>
		<p>Your account has been successfully created, and you have been assigned the role of <strong>%s</strong>.</p>
		<p>You can sign in using the credentials below:</p>
		<p><strong>Email:</strong> %s<br>
		<strong>Temporary Password:</strong> %s<br>
		<strong>Login URL:</strong> <a href="%s">%s</a></p>
		<p>For security reasons, we strongly recommend that you sign in as soon as possible and change your temporary password after your first login.</p>
		<p>If you experience any issues accessing your account, please contact our Vedxlence support team.</p>
		<p>We look forward to having you on Vedxlence.</p>`,
		firstName, roleLabel, email, password, loginURL, loginURL) + emailSignatureHTML
	return
}

// AdminPasswordResetEmail is sent when a staff member resets a user's
// password on their behalf (not the self-service forgot-password OTP flow).
func AdminPasswordResetEmail(firstName, email, password, loginURL string) (subject, html string) {
	subject = "Your Vedxlence Password Has Been Reset"
	html = fmt.Sprintf(`
		<p>Hello %s,</p>
		<p>This is to inform you that the password for your Vedxlence account has been reset by an administrator.</p>
		<p>You can sign in using the credentials below:</p>
		<p><strong>Email:</strong> %s<br>
		<strong>Temporary Password:</strong> %s<br>
		<strong>Login URL:</strong> <a href="%s">%s</a></p>
		<p>For your security, we strongly recommend that you sign in at your earliest convenience and change your temporary password immediately after your first login.</p>
		<p>If you were not expecting this password reset or believe it was made in error, please contact support team as soon as possible.</p>`,
		firstName, email, password, loginURL, loginURL) + emailSignatureHTML
	return
}

// LeaveDecisionEmail is sent when an admin approves or rejects a leave request.
func LeaveDecisionEmail(firstName, status, fromDate, toDate, adminNote string) (subject, html string) {
	noteHTML := ""
	if adminNote != "" {
		noteHTML = fmt.Sprintf(`<p><strong>Administrator's Note:</strong> %s</p>`, adminNote)
	}

	if status == "rejected" {
		subject = "Update on Your Leave Request"
		html = fmt.Sprintf(`
			<p>Hello %s,</p>
			<p>We regret to inform you that your leave request for the period <strong>%s to %s</strong> has been rejected.</p>
			%s
			<p>If you require further clarification or would like to discuss this decision, please contact your manager or the HR team.</p>`,
			firstName, fromDate, toDate, noteHTML) + emailSignatureHTML
		return
	}

	subject = "Your Leave Request Has Been Approved"
	html = fmt.Sprintf(`
		<p>Hello %s,</p>
		<p>We are pleased to inform you that your leave request for the period <strong>%s to %s</strong> has been approved.</p>
		%s
		<p>If you have any questions or require further assistance, please contact your manager or the HR team.</p>`,
		firstName, fromDate, toDate, noteHTML) + emailSignatureHTML
	return
}

func joinLinkHTML(link string) string {
	if link == "" {
		return ""
	}
	return fmt.Sprintf(`<p><strong>Join Link:</strong> <a href="%s">%s</a></p>`, link, link)
}
