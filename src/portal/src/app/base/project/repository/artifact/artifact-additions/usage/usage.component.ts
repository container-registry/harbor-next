// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
import { Component, Input } from '@angular/core';
import { Artifact } from '../../../../../../../../ng-swagger-gen/models/artifact';
import { ArtifactType } from '../../artifact';

// Literal placeholder for the auth credential; never a live secret on a
// standing page. Users mint a robot/view-token and substitute it in.
const TOKEN_PLACEHOLDER = '${TOKEN}';

@Component({
    selector: 'hbr-artifact-usage',
    templateUrl: './usage.component.html',
    styleUrls: ['./usage.component.scss'],
})
export class ArtifactUsageComponent {
    @Input() artifact: Artifact;
    @Input() registryUrl: string;
    @Input() projectName: string;

    get host(): string {
        // npm/maven are served by the same Harbor host the user is browsing, so
        // build setup URLs from the request origin (always client-reachable)
        // rather than a configured internal endpoint.
        return location.origin || this.registryUrl;
    }

    get isNpm(): boolean {
        return this.artifact?.type === ArtifactType.NPM;
    }

    get isMaven(): boolean {
        return this.artifact?.type === ArtifactType.MAVEN;
    }

    private attr(key: string): string {
        const v = this.artifact?.extra_attrs?.[key];
        return typeof v === 'string' ? v : '';
    }

    // ---- npm ----
    get pkgName(): string {
        return this.attr('name');
    }

    get pkgVersion(): string {
        return this.attr('version');
    }

    get scope(): string {
        const name = this.pkgName;
        if (name.startsWith('@') && name.includes('/')) {
            return name.substring(0, name.indexOf('/'));
        }
        return '';
    }

    get npmRegistryUrl(): string {
        return `${this.host}/npm/${this.projectName}/`;
    }

    // The host:port portion (no scheme) used as the _authToken key.
    private get authHost(): string {
        return this.host.replace(/^https?:\/\//, '');
    }

    get npmrcSnippet(): string {
        const authLine = `//${this.authHost}/npm/${this.projectName}/:_authToken=${TOKEN_PLACEHOLDER}`;
        if (this.scope) {
            return `${this.scope}:registry=${this.npmRegistryUrl}\n${authLine}`;
        }
        return `registry=${this.npmRegistryUrl}\n${authLine}`;
    }

    get npmInstallSnippet(): string {
        if (this.pkgVersion) {
            return `npm install ${this.pkgName}@${this.pkgVersion}`;
        }
        return `npm install ${this.pkgName}`;
    }

    // ---- maven ----
    get groupId(): string {
        return this.attr('groupId');
    }

    get artifactId(): string {
        return this.attr('artifactId');
    }

    get mavenVersion(): string {
        return this.attr('version');
    }

    get mavenRepoUrl(): string {
        return `${this.host}/maven/${this.projectName}/`;
    }

    get serverXml(): string {
        return [
            '<server>',
            '  <id>harbor</id>',
            '  <username>robot-account</username>',
            `  <password>${TOKEN_PLACEHOLDER}</password>`,
            '</server>',
        ].join('\n');
    }

    get repoXml(): string {
        return [
            '<repository>',
            '  <id>harbor</id>',
            `  <url>${this.mavenRepoUrl}</url>`,
            '</repository>',
        ].join('\n');
    }

    get dependencyXml(): string {
        return [
            '<dependency>',
            `  <groupId>${this.groupId}</groupId>`,
            `  <artifactId>${this.artifactId}</artifactId>`,
            `  <version>${this.mavenVersion}</version>`,
            '</dependency>',
        ].join('\n');
    }
}
