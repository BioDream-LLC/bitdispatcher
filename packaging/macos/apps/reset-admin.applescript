-- Reset admin password.app: asks, asks macOS for an administrator, runs reset-admin.sh, shows the password.
set ic to path to resource "applet.icns"
activate
display dialog "Set a new password for the bitdispatcher user \"admin\"?" & return & return & "The old password stops working. The service stops for a few seconds, so consoles reconnect." with title "bitdispatcher: reset admin password" buttons {"Cancel", "Reset"} default button "Cancel" cancel button "Cancel" with icon ic
try
	set out to do shell script "/usr/local/bitdispatcher/reset-admin.sh" with administrator privileges
on error e number n
	if n is -128 then return
	display dialog "The password was not reset:" & return & return & e with title "bitdispatcher" buttons {"OK"} default button "OK" with icon stop
	return
end try
set pw to ""
repeat with l in paragraphs of out
	if l starts with "New password for admin: " then set pw to text 25 thru -1 of l
end repeat
if pw is "" then
	display dialog "The password was not reset:" & return & return & out with title "bitdispatcher" buttons {"OK"} default button "OK" with icon stop
	return
end if
set msg to "The new password for admin:" & return & return & pw & return & return & "It is shown only now."
repeat
	set r to display dialog msg with title "bitdispatcher: reset admin password" buttons {"Copy password", "Done"} default button "Done" with icon ic
	if button returned of r is "Done" then exit repeat
	set the clipboard to pw
	set msg to "The password is copied." & return & return & msg
end repeat
