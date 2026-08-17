#!/usr/bin/env bash
set -euo pipefail

suite="${1:-${SUITE:-all}}"

if [[ "${suite}" == "shell" ]]; then
  exec bash -l
fi

: "${HARBOR_MAVEN_URL:?set HARBOR_MAVEN_URL, for example http://core:8080/maven/library/}"
: "${HARBOR_USERNAME:?set HARBOR_USERNAME}"
: "${HARBOR_PASSWORD:?set HARBOR_PASSWORD}"

repo_id="${HARBOR_REPOSITORY_ID:-harbor}"
group_id="${HARBOR_MAVEN_GROUP:-com.harbor.maven.compat}"
workdir="${WORKDIR:-/work/compat}"
run_id="${RUN_ID:-$(date -u +%Y%m%d%H%M%S)}"
release_version="${MAVEN_FIXTURE_VERSION:-1.0.${run_id}}"
snapshot_version="${MAVEN_SNAPSHOT_VERSION:-1.0.${run_id}-SNAPSHOT}"
large_mb="${LARGE_MB:-30}"
repo_url="${HARBOR_MAVEN_URL%/}/"
settings="${workdir}/settings.xml"
deploy_m2="${workdir}/.m2-deploy"
resolve_m2="${workdir}/.m2-resolve"

rm -rf "${workdir}"
mkdir -p "${workdir}"
cd "${workdir}"

cat > "${settings}" <<XML
<settings xmlns="http://maven.apache.org/SETTINGS/1.0.0"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xsi:schemaLocation="http://maven.apache.org/SETTINGS/1.0.0 https://maven.apache.org/xsd/settings-1.0.0.xsd">
  <servers>
    <server>
      <id>${repo_id}</id>
      <username>${HARBOR_USERNAME}</username>
      <password>${HARBOR_PASSWORD}</password>
    </server>
  </servers>
</settings>
XML

if [[ "${MIRROR_ALL_TO_HARBOR:-false}" == "true" ]]; then
  cat > "${settings}" <<XML
<settings xmlns="http://maven.apache.org/SETTINGS/1.0.0"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xsi:schemaLocation="http://maven.apache.org/SETTINGS/1.0.0 https://maven.apache.org/xsd/settings-1.0.0.xsd">
  <servers>
    <server>
      <id>${repo_id}</id>
      <username>${HARBOR_USERNAME}</username>
      <password>${HARBOR_PASSWORD}</password>
    </server>
  </servers>
  <mirrors>
    <mirror>
      <id>${repo_id}</id>
      <url>${repo_url}</url>
      <mirrorOf>*</mirrorOf>
    </mirror>
  </mirrors>
</settings>
XML
fi

log() {
  printf '\n==> %s\n' "$*"
}

mvn_deploy() {
  mvn -B -s "${settings}" -Dmaven.repo.local="${deploy_m2}" "$@"
}

mvn_resolve() {
  mvn -B -s "${settings}" -Dmaven.repo.local="${resolve_m2}" "$@"
}

distribution_management() {
  cat <<XML
  <distributionManagement>
    <repository>
      <id>${repo_id}</id>
      <url>${repo_url}</url>
    </repository>
    <snapshotRepository>
      <id>${repo_id}</id>
      <url>${repo_url}</url>
    </snapshotRepository>
  </distributionManagement>
XML
}

common_metadata() {
  local artifact="$1"
  cat <<XML
  <name>${artifact}</name>
  <description>Harbor Maven compatibility fixture ${artifact}</description>
  <url>https://github.com/container-registry/8gcr</url>
  <licenses>
    <license>
      <name>Apache License, Version 2.0</name>
      <url>https://www.apache.org/licenses/LICENSE-2.0.txt</url>
    </license>
  </licenses>
  <developers>
    <developer>
      <id>harbor-compat</id>
      <name>Harbor Compatibility Fixture</name>
    </developer>
  </developers>
  <scm>
    <connection>scm:git:https://github.com/container-registry/8gcr.git</connection>
    <developerConnection>scm:git:ssh://git@github.com/container-registry/8gcr.git</developerConnection>
    <url>https://github.com/container-registry/8gcr</url>
  </scm>
XML
}

common_properties() {
  cat <<XML
  <properties>
    <maven.compiler.source>8</maven.compiler.source>
    <maven.compiler.target>8</maven.compiler.target>
    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
  </properties>
XML
}

write_java_project() {
  local dir="$1"
  local artifact="$2"
  local version="$3"
  local packaging="${4:-jar}"
  local extra="${5:-}"
  mkdir -p "${dir}/src/main/java/com/harbor/compat" "${dir}/src/main/resources"
  cat > "${dir}/src/main/java/com/harbor/compat/App.java" <<JAVA
package com.harbor.compat;

public final class App {
  private App() {}
  public static String name() { return "${artifact}"; }
}
JAVA
  cat > "${dir}/pom.xml" <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>${group_id}</groupId>
  <artifactId>${artifact}</artifactId>
  <version>${version}</version>
  <packaging>${packaging}</packaging>
$(common_metadata "${artifact}")
$(common_properties)
$(distribution_management)
  ${extra}
</project>
XML
}

deploy() {
  local dir="$1"
  log "Deploying ${dir}"
  (cd "${dir}" && mvn_deploy deploy -DskipTests)
}

write_basic_projects() {
  write_java_project tiny-lib tiny-lib "${release_version}"

  write_java_project medium-resource-lib medium-resource-lib "${release_version}"
  dd if=/dev/zero of=medium-resource-lib/src/main/resources/medium.bin bs=1M count=5 status=none

  write_java_project large-resource-lib large-resource-lib "${release_version}"
  dd if=/dev/zero of=large-resource-lib/src/main/resources/large.bin bs=1M count="${large_mb}" status=none

  write_java_project snapshot-lib snapshot-lib "${snapshot_version}"

  write_java_project pom-only-parent pom-only-parent "${release_version}" pom
}

write_classifier_projects() {
  write_java_project sources-javadoc-lib sources-javadoc-lib "${release_version}" jar '
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-source-plugin</artifactId>
        <version>3.3.1</version>
        <executions>
          <execution>
            <id>attach-sources</id>
            <goals><goal>jar-no-fork</goal></goals>
          </execution>
        </executions>
      </plugin>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-javadoc-plugin</artifactId>
        <version>3.7.0</version>
        <executions>
          <execution>
            <id>attach-javadocs</id>
            <goals><goal>jar</goal></goals>
          </execution>
        </executions>
      </plugin>
    </plugins>
  </build>'

  write_java_project attached-zip-lib attached-zip-lib "${release_version}" jar '
  <build>
    <plugins>
      <plugin>
        <artifactId>maven-assembly-plugin</artifactId>
        <version>3.7.1</version>
        <configuration>
          <descriptorRefs><descriptorRef>project</descriptorRef></descriptorRefs>
          <formats><format>zip</format></formats>
        </configuration>
        <executions>
          <execution>
            <id>attach-assembly</id>
            <phase>package</phase>
            <goals><goal>single</goal></goals>
          </execution>
        </executions>
      </plugin>
    </plugins>
  </build>'

  write_java_project native-classifier-lib native-classifier-lib "${release_version}" jar '
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-antrun-plugin</artifactId>
        <version>3.1.0</version>
        <executions>
          <execution>
            <id>create-native-zip</id>
            <phase>generate-resources</phase>
            <configuration>
              <target>
                <mkdir dir="${project.build.directory}/native"/>
                <echo file="${project.build.directory}/native/README.txt">native fixture</echo>
                <zip destfile="${project.build.directory}/${project.artifactId}-${project.version}-linux-x86_64.zip" basedir="${project.build.directory}/native"/>
              </target>
            </configuration>
            <goals><goal>run</goal></goals>
          </execution>
        </executions>
      </plugin>
      <plugin>
        <groupId>org.codehaus.mojo</groupId>
        <artifactId>build-helper-maven-plugin</artifactId>
        <version>3.6.0</version>
        <executions>
          <execution>
            <id>attach-native-zip</id>
            <phase>package</phase>
            <goals><goal>attach-artifact</goal></goals>
            <configuration>
              <artifacts>
                <artifact>
                  <file>${project.build.directory}/${project.artifactId}-${project.version}-linux-x86_64.zip</file>
                  <type>zip</type>
                  <classifier>linux-x86_64</classifier>
                </artifact>
              </artifacts>
            </configuration>
          </execution>
        </executions>
      </plugin>
    </plugins>
  </build>'
}

write_web_project() {
  write_java_project war-app war-app "${release_version}" war '
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-war-plugin</artifactId>
        <version>3.4.0</version>
        <configuration><failOnMissingWebXml>false</failOnMissingWebXml></configuration>
      </plugin>
    </plugins>
  </build>'
  mkdir -p war-app/src/main/webapp
  printf '<html>harbor maven compat</html>\n' > war-app/src/main/webapp/index.html
}

write_multi_module_project() {
  mkdir -p complex-reactor/{platform-bom,core-api,core-impl,cli-app,web-app}/src/main/java/com/harbor/compat
  cat > complex-reactor/pom.xml <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>${group_id}</groupId>
  <artifactId>complex-reactor</artifactId>
  <version>${release_version}</version>
  <packaging>pom</packaging>
$(common_metadata "complex-reactor")
$(common_properties)
$(distribution_management)
  <modules>
    <module>platform-bom</module>
    <module>core-api</module>
    <module>core-impl</module>
    <module>cli-app</module>
    <module>web-app</module>
  </modules>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>com.google.guava</groupId>
        <artifactId>guava</artifactId>
        <version>33.3.1-jre</version>
      </dependency>
      <dependency>
        <groupId>org.apache.commons</groupId>
        <artifactId>commons-lang3</artifactId>
        <version>3.17.0</version>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <build>
    <pluginManagement>
      <plugins>
        <plugin>
          <groupId>org.apache.maven.plugins</groupId>
          <artifactId>maven-war-plugin</artifactId>
          <version>3.4.0</version>
        </plugin>
      </plugins>
    </pluginManagement>
  </build>
</project>
XML

  cat > complex-reactor/platform-bom/pom.xml <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <parent>
    <groupId>${group_id}</groupId>
    <artifactId>complex-reactor</artifactId>
    <version>${release_version}</version>
  </parent>
  <artifactId>platform-bom</artifactId>
  <packaging>pom</packaging>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>${group_id}</groupId>
        <artifactId>core-api</artifactId>
        <version>${release_version}</version>
      </dependency>
      <dependency>
        <groupId>${group_id}</groupId>
        <artifactId>core-impl</artifactId>
        <version>${release_version}</version>
      </dependency>
    </dependencies>
  </dependencyManagement>
</project>
XML

  cat > complex-reactor/core-api/pom.xml <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <parent>
    <groupId>${group_id}</groupId>
    <artifactId>complex-reactor</artifactId>
    <version>${release_version}</version>
  </parent>
  <artifactId>core-api</artifactId>
  <dependencies>
    <dependency>
      <groupId>org.apache.commons</groupId>
      <artifactId>commons-lang3</artifactId>
    </dependency>
  </dependencies>
</project>
XML
  cat > complex-reactor/core-api/src/main/java/com/harbor/compat/MessageApi.java <<JAVA
package com.harbor.compat;
public interface MessageApi { String message(); }
JAVA

  cat > complex-reactor/core-impl/pom.xml <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <parent>
    <groupId>${group_id}</groupId>
    <artifactId>complex-reactor</artifactId>
    <version>${release_version}</version>
  </parent>
  <artifactId>core-impl</artifactId>
  <dependencies>
    <dependency>
      <groupId>${group_id}</groupId>
      <artifactId>core-api</artifactId>
      <version>${release_version}</version>
    </dependency>
    <dependency>
      <groupId>com.google.guava</groupId>
      <artifactId>guava</artifactId>
    </dependency>
  </dependencies>
</project>
XML
  cat > complex-reactor/core-impl/src/main/java/com/harbor/compat/MessageImpl.java <<JAVA
package com.harbor.compat;
import com.google.common.base.Joiner;
public final class MessageImpl implements MessageApi {
  public String message() { return Joiner.on("-").join("harbor", "maven", "compat"); }
}
JAVA

  cat > complex-reactor/cli-app/pom.xml <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <parent>
    <groupId>${group_id}</groupId>
    <artifactId>complex-reactor</artifactId>
    <version>${release_version}</version>
  </parent>
  <artifactId>cli-app</artifactId>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>${group_id}</groupId>
        <artifactId>platform-bom</artifactId>
        <version>${release_version}</version>
        <type>pom</type>
        <scope>import</scope>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <dependencies>
    <dependency>
      <groupId>${group_id}</groupId>
      <artifactId>core-impl</artifactId>
    </dependency>
  </dependencies>
</project>
XML
  cat > complex-reactor/cli-app/src/main/java/com/harbor/compat/CliApp.java <<JAVA
package com.harbor.compat;
public final class CliApp { public static void main(String[] args) { System.out.println(new MessageImpl().message()); } }
JAVA

  cat > complex-reactor/web-app/pom.xml <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <parent>
    <groupId>${group_id}</groupId>
    <artifactId>complex-reactor</artifactId>
    <version>${release_version}</version>
  </parent>
  <artifactId>web-app</artifactId>
  <packaging>war</packaging>
  <dependencies>
    <dependency>
      <groupId>${group_id}</groupId>
      <artifactId>core-api</artifactId>
      <version>${release_version}</version>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-war-plugin</artifactId>
        <configuration><failOnMissingWebXml>false</failOnMissingWebXml></configuration>
      </plugin>
    </plugins>
  </build>
</project>
XML
  mkdir -p complex-reactor/web-app/src/main/webapp
  printf '<html>complex reactor</html>\n' > complex-reactor/web-app/src/main/webapp/index.html
}

write_plugin_project() {
  mkdir -p compat-maven-plugin/src/main/java/com/harbor/compat/plugin
  cat > compat-maven-plugin/pom.xml <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>${group_id}</groupId>
  <artifactId>compat-maven-plugin</artifactId>
  <version>${release_version}</version>
  <packaging>maven-plugin</packaging>
$(common_metadata "compat-maven-plugin")
$(common_properties)
$(distribution_management)
  <dependencies>
    <dependency>
      <groupId>org.apache.maven</groupId>
      <artifactId>maven-plugin-api</artifactId>
      <version>3.9.9</version>
      <scope>provided</scope>
    </dependency>
    <dependency>
      <groupId>org.apache.maven.plugin-tools</groupId>
      <artifactId>maven-plugin-annotations</artifactId>
      <version>3.13.1</version>
      <scope>provided</scope>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-plugin-plugin</artifactId>
        <version>3.13.1</version>
        <executions>
          <execution>
            <id>descriptor</id>
            <goals><goal>descriptor</goal></goals>
          </execution>
        </executions>
      </plugin>
    </plugins>
  </build>
</project>
XML
  cat > compat-maven-plugin/src/main/java/com/harbor/compat/plugin/TouchMojo.java <<JAVA
package com.harbor.compat.plugin;

import java.io.File;
import java.io.FileWriter;
import java.io.IOException;
import org.apache.maven.plugin.AbstractMojo;
import org.apache.maven.plugin.MojoExecutionException;
import org.apache.maven.plugins.annotations.Mojo;
import org.apache.maven.plugins.annotations.Parameter;

@Mojo(name = "touch")
public final class TouchMojo extends AbstractMojo {
  @Parameter(property = "touch.file", defaultValue = "\${project.build.directory}/harbor-plugin-ran.txt")
  private File file;

  public void execute() throws MojoExecutionException {
    try {
      File parent = file.getParentFile();
      if (parent != null) {
        parent.mkdirs();
      }
      try (FileWriter writer = new FileWriter(file)) {
        writer.write("ok\\n");
      }
    } catch (IOException e) {
      throw new MojoExecutionException("failed to write marker", e);
    }
  }
}
JAVA
}

write_archetype_project() {
  mkdir -p compat-archetype/src/main/resources/META-INF/maven compat-archetype/src/main/resources/archetype-resources/src/main/java
  cat > compat-archetype/pom.xml <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>${group_id}</groupId>
  <artifactId>compat-archetype</artifactId>
  <version>${release_version}</version>
  <packaging>maven-archetype</packaging>
$(common_metadata "compat-archetype")
$(common_properties)
$(distribution_management)
  <build>
    <extensions>
      <extension>
        <groupId>org.apache.maven.archetype</groupId>
        <artifactId>archetype-packaging</artifactId>
        <version>3.3.1</version>
      </extension>
    </extensions>
  </build>
</project>
XML
  cat > compat-archetype/src/main/resources/META-INF/maven/archetype-metadata.xml <<XML
<archetype-descriptor name="compat-archetype">
  <fileSets>
    <fileSet filtered="true" packaged="true">
      <directory>src/main/java</directory>
      <includes>
        <include>**/*.java</include>
      </includes>
    </fileSet>
  </fileSets>
</archetype-descriptor>
XML
  cat > compat-archetype/src/main/resources/archetype-resources/pom.xml <<'XML'
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>${groupId}</groupId>
  <artifactId>${artifactId}</artifactId>
  <version>${version}</version>
</project>
XML
  cat > compat-archetype/src/main/resources/archetype-resources/src/main/java/App.java <<'JAVA'
package ${package};
public final class App { public static String name() { return "${artifactId}"; } }
JAVA
}

write_manual_artifacts() {
  mkdir -p manual-artifacts
  printf 'vendor binary fixture\n' > manual-artifacts/vendor.txt
  zip -q -j manual-artifacts/vendor-sdk-${release_version}-linux-x86_64.zip manual-artifacts/vendor.txt
  cat > manual-artifacts/vendor-sdk.pom <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>${group_id}</groupId>
  <artifactId>vendor-sdk</artifactId>
  <version>${release_version}</version>
  <packaging>zip</packaging>
$(common_metadata "vendor-sdk")
</project>
XML
}

write_consumer_project() {
  write_java_project dependency-consumer-lib dependency-consumer-lib "${release_version}" jar "
  <dependencies>
    <dependency>
      <groupId>${group_id}</groupId>
      <artifactId>tiny-lib</artifactId>
      <version>${release_version}</version>
    </dependency>
    <dependency>
      <groupId>${group_id}</groupId>
      <artifactId>snapshot-lib</artifactId>
      <version>${snapshot_version}</version>
    </dependency>
    <dependency>
      <groupId>${group_id}</groupId>
      <artifactId>core-api</artifactId>
      <version>${release_version}</version>
    </dependency>
  </dependencies>"
}

write_resolve_check() {
  rm -rf "${resolve_m2}"
  mkdir -p resolve-check
  cat > resolve-check/pom.xml <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>${group_id}</groupId>
  <artifactId>resolve-check</artifactId>
  <version>${release_version}</version>
$(common_properties)
  <repositories>
    <repository>
      <id>${repo_id}</id>
      <url>${repo_url}</url>
      <releases><enabled>true</enabled><updatePolicy>always</updatePolicy></releases>
      <snapshots><enabled>true</enabled><updatePolicy>always</updatePolicy></snapshots>
    </repository>
  </repositories>
  <pluginRepositories>
    <pluginRepository>
      <id>${repo_id}</id>
      <url>${repo_url}</url>
      <releases><enabled>true</enabled><updatePolicy>always</updatePolicy></releases>
      <snapshots><enabled>true</enabled><updatePolicy>always</updatePolicy></snapshots>
    </pluginRepository>
  </pluginRepositories>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>${group_id}</groupId>
        <artifactId>platform-bom</artifactId>
        <version>${release_version}</version>
        <type>pom</type>
        <scope>import</scope>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <dependencies>
    <dependency><groupId>${group_id}</groupId><artifactId>tiny-lib</artifactId><version>${release_version}</version></dependency>
    <dependency><groupId>${group_id}</groupId><artifactId>snapshot-lib</artifactId><version>${snapshot_version}</version></dependency>
    <dependency><groupId>${group_id}</groupId><artifactId>dependency-consumer-lib</artifactId><version>${release_version}</version></dependency>
    <dependency><groupId>${group_id}</groupId><artifactId>core-impl</artifactId></dependency>
  </dependencies>
</project>
XML
  log "Resolving deployed artifacts from a clean local repository"
  (cd resolve-check && mvn_resolve -U dependency:go-offline)
  mvn_resolve dependency:get -DremoteRepositories="${repo_id}::default::${repo_url}" -Dartifact="${group_id}:sources-javadoc-lib:${release_version}:jar:sources"
  mvn_resolve dependency:get -DremoteRepositories="${repo_id}::default::${repo_url}" -Dartifact="${group_id}:native-classifier-lib:${release_version}:zip:linux-x86_64"
  mvn_resolve dependency:get -DremoteRepositories="${repo_id}::default::${repo_url}" -Dartifact="${group_id}:vendor-sdk:${release_version}:zip:linux-x86_64"
}

run_basic() {
  write_basic_projects
  for project in tiny-lib medium-resource-lib large-resource-lib snapshot-lib pom-only-parent; do
    deploy "${project}"
  done
}

run_classifiers() {
  write_classifier_projects
  for project in sources-javadoc-lib attached-zip-lib native-classifier-lib; do
    deploy "${project}"
  done
}

run_web() {
  write_web_project
  deploy war-app
}

run_multi() {
  write_multi_module_project
  deploy complex-reactor
}

run_plugin() {
  write_plugin_project
  deploy compat-maven-plugin
  mkdir -p plugin-consumer
  cat > plugin-consumer/pom.xml <<XML
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>${group_id}</groupId>
  <artifactId>plugin-consumer</artifactId>
  <version>${release_version}</version>
  <pluginRepositories>
    <pluginRepository>
      <id>${repo_id}</id>
      <url>${repo_url}</url>
    </pluginRepository>
  </pluginRepositories>
</project>
XML
  log "Resolving and running deployed Maven plugin by full coordinate"
  (cd plugin-consumer && mvn_resolve -U "${group_id}:compat-maven-plugin:${release_version}:touch" -Dtouch.file=target/plugin-ran.txt)
}

run_archetype() {
  write_archetype_project
  deploy compat-archetype
  log "Resolving deployed Maven archetype by GAV"
  mvn_resolve dependency:get -DremoteRepositories="${repo_id}::default::${repo_url}" -Dartifact="${group_id}:compat-archetype:${release_version}:jar"
}

run_manual() {
  write_manual_artifacts
  log "Deploying manual vendor artifact with classifier and supplied POM"
  mvn_deploy deploy:deploy-file \
    -DrepositoryId="${repo_id}" \
    -Durl="${repo_url}" \
    -DgroupId="${group_id}" \
    -DartifactId=vendor-sdk \
    -Dversion="${release_version}" \
    -Dpackaging=zip \
    -Dclassifier=linux-x86_64 \
    -Dfile="manual-artifacts/vendor-sdk-${release_version}-linux-x86_64.zip" \
    -DpomFile="manual-artifacts/vendor-sdk.pom"
}

run_resolve() {
  run_basic
  run_classifiers
  run_multi
  run_manual
  run_web
  write_consumer_project
  deploy dependency-consumer-lib
  write_resolve_check
}

run_all() {
  run_basic
  run_classifiers
  run_web
  run_multi
  run_plugin
  run_archetype
  run_manual
  write_consumer_project
  deploy dependency-consumer-lib
  write_resolve_check
}

case "${suite}" in
  all) run_all ;;
  basic) run_basic ;;
  classifiers) run_classifiers ;;
  web) run_web ;;
  multi) run_multi ;;
  plugin) run_plugin ;;
  archetype) run_archetype ;;
  manual) run_manual ;;
  resolve) run_resolve ;;
  *)
    echo "unknown suite: ${suite}" >&2
    echo "valid suites: all, basic, classifiers, web, multi, plugin, archetype, manual, resolve, shell" >&2
    exit 2
    ;;
esac

log "Maven compatibility run completed"
echo "Repository URL: ${repo_url}"
echo "Group ID: ${group_id}"
echo "Release version: ${release_version}"
echo "Snapshot version: ${snapshot_version}"
