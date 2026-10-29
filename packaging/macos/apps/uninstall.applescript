-- Uninstall bitdispatcher.app: asks, then whether to keep the records (kept unless confirmed twice).
set ic to path to resource "applet.icns"
activate
display dialog "Remove bitdispatcher from this Mac?" & return & return & "The service stops and consoles can no longer connect." with title "Uninstall bitdispatcher" buttons {"Cancel", "Uninstall"} default button "Cancel" cancel button "Cancel" with icon ic
set how to "--keep"
set r to display dialog "Also delete the records?" & return & return & "They include the ledger of every call and dispatch. Keep them unless you have a copy and your retention rules allow it." with title "Uninstall bitdispatcher" buttons {"Delete records", "Keep records"} default button "Keep records" with icon caution
if button returned of r is "Delete records" then
	set r to display dialog "Delete the records permanently? This cannot be undone." with title "Uninstall bitdispatcher" buttons {"Delete permanently", "Keep records"} default button "Keep records" with icon caution
	if button returned of r is "Delete permanently" then set how to "--purge-confirmed"
end if
try
	set out to do shell script "/usr/local/bitdispatcher/uninstall.sh " & how with administrator privileges
on error e number n
	if n is -128 then return
	display dialog "bitdispatcher was not removed:" & return & return & e with title "Uninstall bitdispatcher" buttons {"OK"} default button "OK" with icon stop
	return
end try
display dialog out with title "Uninstall bitdispatcher" buttons {"OK"} default button "OK"
