-- bitdispatcher.app: opens the console in the default browser, at the address in the settings file.
set addr to do shell script "BITDISPATCHER_ADDR=:8443; [ -r /usr/local/etc/bitdispatcher.conf ] && . /usr/local/etc/bitdispatcher.conf; h=${BITDISPATCHER_ADDR%:*}; case \"$h\" in ''|0.0.0.0|'[::]') h=localhost ;; esac; echo \"https://$h:${BITDISPATCHER_ADDR##*:}/\""
open location addr
