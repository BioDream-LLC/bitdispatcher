#!/bin/sh
# Builds Google's CAP library (Apache-2.0, github.com/google/cap-library, archived) and a small checker
# (scripts/capcheck) into a cache directory, outside this repository, and prints the path of a command
# that checks one CAP message against the CAP 1.2 schema and the library's IPAWS profile:
#
#   CAP_VALIDATOR=$(scripts/cap-validator.sh) go test -run TestValidators ./internal/alerting
#
# Needs git, Java 17 and the internet the first time. Nothing from the library is built into bitdispatcher.
set -eu
HERE=$(cd "$(dirname "$0")" && pwd)
CACHE=${CAP_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/bitdispatcher-cap}
COMMIT=67dcf20373834725b15c5325a73f615d92a708fa
JAXB=https://repo1.maven.org/maven2/javax/xml/bind/jaxb-api/2.3.1/jaxb-api-2.3.1.jar
mkdir -p "$CACHE"
cd "$CACHE"
[ -f jaxb-api-2.3.1.jar ] || curl -sSfL -o jaxb-api-2.3.1.jar "$JAXB"
[ -d cap-library ] || git clone -q https://github.com/google/cap-library.git
(cd cap-library && git checkout -q "$COMMIT")
CP=$(ls "$CACHE"/cap-library/java/lib/*.jar | grep -v -e junit -e jarjar | tr '\n' ':')$CACHE/jaxb-api-2.3.1.jar
echo "$CP" > classpath
if [ ! -f build/com/google/publicalerts/cap/profile/us/Ipaws1Profile.class ]; then
  rm -rf build && mkdir -p build/com/google/publicalerts/cap/schema
  javac -nowarn -encoding UTF-8 --add-exports java.xml/com.sun.org.apache.xerces.internal.util=ALL-UNNAMED -d build -cp "$CP" \
    $(find cap-library/java/src -name '*.java' -not -path '*/testing/*') 2>&1 | grep -E ' error|^[0-9]+ error' >&2 || true
  cp -r cap-library/schema/* build/com/google/publicalerts/cap/schema/
  cp -r cap-library/java/config/* build/ 2>/dev/null || true
  cp cap-library/java/src/com/google/publicalerts/cap/profile/au/*.csv build/com/google/publicalerts/cap/profile/au/ 2>/dev/null || true
fi
javac -nowarn -d build -cp "build:$(cat classpath)" "$HERE/capcheck/CapCheck.java"
cat > validate <<EOS
#!/bin/sh
exec java --add-exports java.xml/com.sun.org.apache.xerces.internal.util=ALL-UNNAMED -cp "$CACHE/build:\$(cat "$CACHE/classpath")" CapCheck "\$1" 2>&1
EOS
chmod +x validate
echo "$CACHE/validate"
