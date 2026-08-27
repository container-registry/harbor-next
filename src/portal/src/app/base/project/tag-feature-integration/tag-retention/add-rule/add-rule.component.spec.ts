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
import { TranslateModule, TranslateService } from '@ngx-translate/core';
import { AddRuleComponent } from './add-rule.component';
import { CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import {
    BrowserAnimationsModule,
    NoopAnimationsModule,
} from '@angular/platform-browser/animations';
import { ClarityModule } from '@clr/angular';
import { FormsModule } from '@angular/forms';
import { RouterTestingModule } from '@angular/router/testing';
import { HttpClientTestingModule } from '@angular/common/http/testing';
import { TagRetentionService } from '../tag-retention.service';
import { ErrorHandler } from '../../../../../shared/units/error-handler';
import { InlineAlertComponent } from '../../../../../shared/components/inline-alert/inline-alert.component';
import { CallbackPipe } from '../../../../../shared/pipes/callback.pipe';
import {
    RuleMetadate,
    SELECTOR_KIND_DOUBLESTAR,
    SELECTOR_KIND_REGEX,
} from '../retention';

describe('AddRuleComponent', () => {
    let component: AddRuleComponent;
    let fixture: ComponentFixture<AddRuleComponent>;
    const mockTagRetentionService = {};

    beforeEach(() => {
        TestBed.configureTestingModule({
            schemas: [CUSTOM_ELEMENTS_SCHEMA],
            imports: [
                BrowserAnimationsModule,
                ClarityModule,
                TranslateModule.forRoot(),
                FormsModule,
                RouterTestingModule,
                NoopAnimationsModule,
                HttpClientTestingModule,
            ],
            declarations: [
                AddRuleComponent,
                CallbackPipe,
                InlineAlertComponent,
            ],
            providers: [
                TranslateService,
                ErrorHandler,
                {
                    provide: TagRetentionService,
                    useValue: mockTagRetentionService,
                },
            ],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(AddRuleComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should default to the doublestar engine', () => {
        expect(component.tagsKind).toEqual(SELECTOR_KIND_DOUBLESTAR);
        expect(component.repoKind).toEqual(SELECTOR_KIND_DOUBLESTAR);
        expect(component.isRegexp(component.tagsKind)).toBeFalsy();
    });

    it('should wrap a comma list in braces for doublestar', () => {
        component.tagsInput = 'v1,v2';
        expect(component.rule.tag_selectors[0].pattern).toEqual('{v1,v2}');
        expect(component.tagsInput).toEqual('v1,v2');

        component.repositories = 'redis,harbor';
        expect(component.rule.scope_selectors.repository[0].pattern).toEqual(
            '{redis,harbor}'
        );
        expect(component.repositories).toEqual('redis,harbor');
    });

    it('should store a regex pattern verbatim', () => {
        component.tagsKind = SELECTOR_KIND_REGEX;

        component.tagsInput = 'v\\d{2,3}';
        expect(component.rule.tag_selectors[0].pattern).toEqual('v\\d{2,3}');
        expect(component.tagsInput).toEqual('v\\d{2,3}');

        // a regex expresses alternation natively, so a comma is content
        component.tagsInput = '(v1|v2),beta';
        expect(component.rule.tag_selectors[0].pattern).toEqual('(v1|v2),beta');

        // and a brace wrapped pattern is not unwrapped on read
        component.tagsInput = '{2,3}';
        expect(component.tagsInput).toEqual('{2,3}');
    });

    it('should keep a brace only regex pattern addable', () => {
        component.rule.template = 'always';
        component.repositories = '**';

        component.tagsKind = SELECTOR_KIND_DOUBLESTAR;
        component.tagsInput = '{}';
        expect(component.canNotAdd()).toBeTruthy();

        component.tagsKind = SELECTOR_KIND_REGEX;
        component.tagsInput = 'v\\d{2}';
        expect(component.canNotAdd()).toBeFalsy();
    });

    it('should drive the engine and decoration options from the metadata', () => {
        component.metadata = new RuleMetadate();
        component.metadata.scope_selectors = [
            {
                display_text: 'Repositories',
                kind: SELECTOR_KIND_DOUBLESTAR,
                decorations: ['repoMatches', 'repoExcludes'],
            },
            {
                display_text: 'Repositories',
                kind: SELECTOR_KIND_REGEX,
                decorations: ['repoMatches', 'repoExcludes'],
            },
        ];
        component.metadata.tag_selectors = [
            {
                display_text: 'Tags',
                kind: SELECTOR_KIND_DOUBLESTAR,
                decorations: ['matches', 'excludes'],
            },
            {
                display_text: 'Tags',
                kind: SELECTOR_KIND_REGEX,
                decorations: ['matches'],
            },
        ];

        expect(component.repoKinds).toEqual([
            SELECTOR_KIND_DOUBLESTAR,
            SELECTOR_KIND_REGEX,
        ]);
        expect(component.tagDecorations).toEqual(['matches', 'excludes']);

        component.tagsSelect = 'excludes';
        component.tagsKind = SELECTOR_KIND_REGEX;
        expect(component.tagDecorations).toEqual(['matches']);
        // the decoration falls back when the new engine does not support it
        expect(component.tagsSelect).toEqual('matches');
    });
});
