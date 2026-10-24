#!/bin/sh
# Sets up EAS Station's CAP poller parser (KR8MER/eas-station, AGPL-3.0) in a cache directory, outside
# this repository, and prints the path of a command that reads an alert feed or CAP message with it:
#
#   EAS_PARSER=$(scripts/eas-station.sh) go test -run TestPublicAlertsAPI ./internal/api
#
# Needs git, uv (for Python 3.12) and the internet the first time. Nothing of EAS Station is in bitdispatcher.
set -eu
HERE=$(cd "$(dirname "$0")" && pwd)
CACHE=${EAS_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/bitdispatcher-eas}
COMMIT=6b52a7019a003d27d860f843d2e082109fc66b68
mkdir -p "$CACHE"
cd "$CACHE"
if [ ! -d eas-station ]; then
  git clone -q https://github.com/KR8MER/eas-station.git
fi
(cd eas-station && git fetch -q --depth 1 origin "$COMMIT" 2>/dev/null || true; git checkout -q "$COMMIT")
if [ ! -x venv/bin/python ]; then
  uv venv -q -p 3.12 venv
  # The parser's imports, at the versions EAS Station pins.
  pins=""
  for p in pytz psutil SQLAlchemy Flask Flask-SQLAlchemy redis GeoAlchemy2 numpy scipy lxml python-dateutil python-dotenv requests certifi; do
    pins="$pins $(grep -iE "^$p(==|>=)" eas-station/requirements.txt | head -1 | cut -d' ' -f1 | sed 's/>=/==/')"
  done
  pins="$pins $(grep -iE '^cryptography(==|>=)' eas-station/requirements.txt | head -1 | cut -d' ' -f1 | sed 's/>=/==/')"
  uv pip install -q -p venv/bin/python $pins
fi
cat > parse <<EOS
#!/bin/sh
cd "$CACHE/eas-station" && EAS_STATION="$CACHE/eas-station" exec "$CACHE/venv/bin/python" "$HERE/eas/easparse.py" "\$1" 2>&1 | grep -v 'not available'
EOS
chmod +x parse
echo "$CACHE/parse"
