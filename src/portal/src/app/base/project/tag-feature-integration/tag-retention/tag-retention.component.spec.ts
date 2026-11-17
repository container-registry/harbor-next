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
import {
    ComponentFixture,
    TestBed,
    fakeAsync,
    tick,
} from '@angular/core/testing';
import { TagRetentionComponent } from './tag-retention.component';
import { CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { of } from 'rxjs';
import { ActivatedRoute } from '@angular/router';
import { AddRuleComponent } from './add-rule/add-rule.component';
import { TagRetentionService } from './tag-retention.service';
import { RuleMetadate, Retention, Rule, TIMEOUT } from './retention';
import { delay } from 'rxjs/operators';
import { SharedTestingModule } from '../../../../shared/shared.module';
import { HttpHeaders, HttpResponse } from '@angular/common/http';
import { Registry } from '../../../../../../ng-swagger-gen/models/registry';
import { RetentionService } from 'ng-swagger-gen/services/retention.service';
import { UserPermissionService } from '../../../../shared/services';
import { ProjectService } from '../../../../../../ng-swagger-gen/services/project.service';

describe('TagRetentionComponent', () => {
    const mockedRunningExecutions = [
        {
            dry_run: true,
            end_time: '2021-04-26T04:32:21Z',
            id: 57,
            policy_id: 1,
            start_time: '2021-04-26T04:32:18.032419Z',
            status: 'Running',
            trigger: 'MANUAL',
        },
    ];
    const mockedSuccessExecutions = [
        {
            dry_run: true,
            end_time: '2021-04-26T04:32:21Z',
            id: 57,
            policy_id: 1,
            start_time: '2021-04-26T04:32:18.032419Z',
            status: 'Success',
            trigger: 'MANUAL',
        },
    ];
    let component: TagRetentionComponent;
    let fixture: ComponentFixture<TagRetentionComponent>;
    const mockTagRetentionService = {
        seeLog: () => of(null).pipe(delay(0)),
        getI18nKey: (key: string) => key,
    };
    const mockRetentionService = {
        createRetention: () => of(null).pipe(delay(0)),
        updateRetention: () => of(null).pipe(delay(0)),
        listRetentionExecutionsResponse() {
            if (this.count === 0) {
                this.count += 1;
                const response: HttpResponse<Array<Registry>> =
                    new HttpResponse<Array<Registry>>({
                        headers: new HttpHeaders({
                            'x-total-count':
                                mockedRunningExecutions.length.toString(),
                        }),
                        body: mockedRunningExecutions,
                    });
                return of(response).pipe(delay(0));
            } else {
                this.count += 1;
                const response: HttpResponse<Array<Registry>> =
                    new HttpResponse<Array<Registry>>({
                        headers: new HttpHeaders({
                            'x-total-count':
                                mockedSuccessExecutions.length.toString(),
                        }),
                        body: mockedSuccessExecutions,
                    });
                return of(response).pipe(delay(0));
            }
        },
        getRentenitionMetadata: () => of(new RuleMetadate()).pipe(delay(0)),
        getRetention: () => of(new Retention()).pipe(delay(0)),
    };
    const mockActivatedRoute = {
        snapshot: {
            parent: {
                parent: {
                    parent: {
                        params: { id: 1 },
                        data: {
                            projectResolver: {
                                metadata: {
                                    retention_id: 1,
                                },
                            },
                        },
                    },
                },
            },
        },
    };
    let permissions = { create: true, update: true };
    const mockUserPermissionService = {
        getPermission(projectId, resource, action) {
            return of(permissions[action]);
        },
    };
    let projectMetadata: { retention_id?: string } = { retention_id: '1' };
    const mockProjectService = {
        getProject: () => of({ metadata: projectMetadata }),
    };
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            schemas: [CUSTOM_ELEMENTS_SCHEMA],
            imports: [SharedTestingModule],
            declarations: [TagRetentionComponent, AddRuleComponent],
            providers: [
                {
                    provide: TagRetentionService,
                    useValue: mockTagRetentionService,
                },
                {
                    provide: RetentionService,
                    useValue: mockRetentionService,
                },
                { provide: ActivatedRoute, useValue: mockActivatedRoute },
                {
                    provide: UserPermissionService,
                    useValue: mockUserPermissionService,
                },
                { provide: ProjectService, useValue: mockProjectService },
            ],
        }).compileComponents();
    });

    beforeEach(() => {
        permissions = { create: true, update: true };
        projectMetadata = { retention_id: '1' };
        fixture = TestBed.createComponent(TagRetentionComponent);
        component = fixture.componentInstance;
        component.loadingRule = false;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should retry getting executions', fakeAsync(() => {
        tick(TIMEOUT);
        fixture.detectChanges();
        fixture.whenStable().then(() => {
            expect(component.executionList[0].status).toEqual('Success');
        });
    }));

    function render(): HTMLElement {
        fixture = TestBed.createComponent(TagRetentionComponent);
        component = fixture.componentInstance;
        component.retention.rules = [new Rule()];
        fixture.detectChanges();
        return fixture.nativeElement;
    }

    function isDisabled(el: HTMLElement, id: string): boolean {
        return el.querySelector<HTMLButtonElement>(id).disabled;
    }

    it('should disable write actions without update permission', () => {
        permissions = { create: true, update: false };
        const el = render();
        expect(isDisabled(el, '#add-rule')).toBeTrue();
        expect(isDisabled(el, '#run-now')).toBeTrue();
        expect(isDisabled(el, '#dry-run')).toBeTrue();
    });

    it('should enable write actions with update permission on an existing policy', () => {
        permissions = { create: false, update: true };
        const el = render();
        expect(isDisabled(el, '#add-rule')).toBeFalse();
        expect(isDisabled(el, '#run-now')).toBeFalse();
        expect(isDisabled(el, '#dry-run')).toBeFalse();
    });

    it('should need create permission for the first rule', () => {
        projectMetadata = {};
        permissions = { create: true, update: false };
        expect(isDisabled(render(), '#add-rule')).toBeFalse();

        permissions = { create: false, update: true };
        expect(isDisabled(render(), '#add-rule')).toBeTrue();
    });
});
