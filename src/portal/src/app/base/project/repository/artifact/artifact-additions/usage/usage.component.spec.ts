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
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { NO_ERRORS_SCHEMA } from '@angular/core';
import { ArtifactUsageComponent } from './usage.component';
import { SharedTestingModule } from '../../../../../../shared/shared.module';

describe('ArtifactUsageComponent', () => {
    let component: ArtifactUsageComponent;
    let fixture: ComponentFixture<ArtifactUsageComponent>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [ArtifactUsageComponent],
            schemas: [NO_ERRORS_SCHEMA],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(ArtifactUsageComponent);
        component = fixture.componentInstance;
        component.registryUrl = 'demo.harbor.io';
        component.projectName = 'library';
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should compute unscoped npm snippets', () => {
        component.artifact = {
            type: 'NPM',
            extra_attrs: { name: 'lodash', version: '4.17.21' },
        } as any;
        fixture.detectChanges();
        expect(component.isNpm).toBeTrue();
        expect(component.scope).toEqual('');
        expect(component.npmInstallSnippet).toEqual(
            'npm install lodash@4.17.21'
        );
        expect(component.npmrcSnippet).toContain(
            `registry=${location.origin}/npm/library/`
        );
        expect(component.npmrcSnippet).toContain(':_authToken=${TOKEN}');
    });

    it('should compute scoped npm registry line', () => {
        component.artifact = {
            type: 'NPM',
            extra_attrs: { name: '@acme/widget', version: '1.0.0' },
        } as any;
        fixture.detectChanges();
        expect(component.scope).toEqual('@acme');
        expect(component.npmrcSnippet).toContain('@acme:registry=');
    });

    it('should compute maven snippets', () => {
        component.artifact = {
            type: 'MAVEN',
            extra_attrs: {
                groupId: 'com.acme',
                artifactId: 'widget2',
                version: '1.0',
            },
        } as any;
        fixture.detectChanges();
        expect(component.isMaven).toBeTrue();
        expect(component.serverXml).toContain('<id>harbor</id>');
        expect(component.serverXml).toContain('<password>${TOKEN}</password>');
        expect(component.repoXml).toContain(
            `${location.origin}/maven/library/`
        );
        expect(component.dependencyXml).toContain(
            '<artifactId>widget2</artifactId>'
        );
    });
});
