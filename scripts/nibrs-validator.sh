#!/bin/sh
# Builds SEARCH's NIBRS Pre-Certification Tool validator (Apache-2.0, github.com/mark43/nibrs, an archived
# copy of SEARCH-NCJIS/nibrs) into a cache directory, outside this repository, and prints the path of a
# command that validates one FBI-layout NIBRS flat file and prints SEARCH's error report:
#
#   NIBRS_VALIDATOR=$(scripts/nibrs-validator.sh) go test -run TestSEARCHValidator ./internal/ibr
#
# Needs git, Java 17 and the internet the first time. Nothing from the toolkit is built into bitdispatcher.
set -eu
CACHE=${NIBRS_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/bitdispatcher-nibrs}
COMMIT=65dcd57f874f8211a9fb507fbfd082ec9177b669
MAVEN=3.9.9
mkdir -p "$CACHE"
cd "$CACHE"
if [ ! -x "apache-maven-$MAVEN/bin/mvn" ]; then
  curl -sSfL "https://archive.apache.org/dist/maven/maven-3/$MAVEN/binaries/apache-maven-$MAVEN-bin.tar.gz" | tar xz
fi
MVN="$CACHE/apache-maven-$MAVEN/bin/mvn -q -B -Dmaven.repo.local=$CACHE/m2 -Dlicense.skip=true -DskipTests"
if [ ! -f nibrs/tools/nibrs-apps/target/nibrs-apps-0.0.1-SNAPSHOT.jar ]; then
  [ -d nibrs ] || git clone -q https://github.com/mark43/nibrs.git
  (cd nibrs && git checkout -q "$COMMIT")
  (cd nibrs/shared/nibrs-build-common && $MVN install) >&2
  (cd nibrs && $MVN -N install && cd tools && $MVN -N install) >&2
  (cd nibrs/tools && $MVN -pl nibrs-common,nibrs-resources,nibrs-flatfile,nibrs-validation,nibrs-xmlfile,nibrs-apps -am install) >&2
  (cd nibrs/tools/nibrs-apps && $MVN dependency:build-classpath -Dmdep.outputFile="$CACHE/classpath") >&2
fi
cat > validate <<EOF
#!/bin/sh
exec java -cp "$CACHE/nibrs/tools/nibrs-apps/target/nibrs-apps-0.0.1-SNAPSHOT.jar:\$(cat "$CACHE/classpath")" org.search.nibrs.apps.NIBRSValidator -f "\$1" 2>/dev/null
EOF
chmod +x validate
echo "$CACHE/validate"
