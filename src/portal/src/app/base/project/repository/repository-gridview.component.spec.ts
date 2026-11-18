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
import { AppConfigService } from '../../../services/app-config.service';
import {
    ComponentFixture,
    discardPeriodicTasks,
    fakeAsync,
    TestBed,
    tick,
} from '@angular/core/testing';
import { BehaviorSubject, of } from 'rxjs';
import { RepositoryService as NewRepositoryService } from '../../../../../ng-swagger-gen/services/repository.service';
import { RepositoryGridviewComponent } from './repository-gridview.component';
import {
    ProjectDefaultService,
    ProjectService,
    SystemInfo,
    SystemInfoService,
    UserPermissionService,
} from '../../../shared/services';
import { delay } from 'rxjs/operators';
import { ErrorHandler } from '../../../shared/units/error-handler';
import { ActivatedRoute, convertToParamMap, ParamMap } from '@angular/router';
import { Repository as NewRepository } from '../../../../../ng-swagger-gen/models/repository';
import { SharedTestingModule } from '../../../shared/shared.module';
import { GridViewComponent } from './gridview/grid-view.component';

describe('RepositoryComponentGridview (inline template)', () => {
    let compRepo: RepositoryGridviewComponent;
    let fixtureRepo: ComponentFixture<RepositoryGridviewComponent>;
    let mockSystemInfo: SystemInfo = {
        with_admiral: false,
        admiral_endpoint: 'NA',
        auth_mode: 'db_auth',
        registry_url: '10.112.122.56',
        project_creation_restriction: 'everyone',
        self_registration: true,
        has_ca_root: false,
        harbor_version: 'v1.1.1-rc1-160-g565110d',
    };
    let mockRepoData: NewRepository[] = [
        {
            id: 1,
            name: 'library/busybox',
            project_id: 1,
            description: 'asdfsadf',
            pull_count: 0,
            artifact_count: 1,
        },
        {
            id: 2,
            name: 'library/nginx',
            project_id: 1,
            description: 'asdf',
            pull_count: 0,
            artifact_count: 1,
        },
    ];
    let mockRepoNginxData: NewRepository[] = [
        {
            id: 2,
            name: 'library/nginx',
            project_id: 1,
            description: 'asdf',
            pull_count: 0,
            artifact_count: 1,
        },
    ];

    let mockRepo: NewRepository[] = mockRepoData;
    let mockNginxRepo: NewRepository[] = mockRepoNginxData;
    const fakedErrorHandler = {
        error() {
            return undefined;
        },
    };
    const fakedSystemInfoService = {
        getSystemInfo() {
            return of(mockSystemInfo);
        },
    };
    const fakedRepositoryService = {
        listRepositoriesResponse(
            params: NewRepositoryService.ListRepositoriesParams
        ) {
            if (params.q === encodeURIComponent(`name=~nginx`)) {
                return of({ headers: new Map(), body: mockNginxRepo });
            }
            return of({ headers: new Map(), body: mockRepo }).pipe(delay(0));
        },
    };
    const fakedUserPermissionService = {
        getPermission() {
            return of(true);
        },
    };
    let queryParamMap: BehaviorSubject<ParamMap>;
    const fakedActivatedRoute = {
        get queryParamMap() {
            return queryParamMap;
        },
        snapshot: {
            queryParams: {},
            parent: {
                parent: {
                    params: {
                        id: '1',
                    },
                },
            },
            queryParamMap: convertToParamMap({}),
        },
    };
    beforeEach(() => {
        queryParamMap = new BehaviorSubject(convertToParamMap({}));
        let store = {};
        spyOn(localStorage, 'getItem').and.callFake(key => {
            return store[key];
        });
        spyOn(localStorage, 'setItem').and.callFake((key, value) => {
            return (store[key] = value + '');
        });
        spyOn(localStorage, 'clear').and.callFake(() => {
            store = {};
        });
        TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [RepositoryGridviewComponent, GridViewComponent],
            providers: [
                { provide: ActivatedRoute, useValue: fakedActivatedRoute },
                { provide: ErrorHandler, useValue: fakedErrorHandler },
                {
                    provide: NewRepositoryService,
                    useValue: fakedRepositoryService,
                },
                { provide: ProjectService, useClass: ProjectDefaultService },
                {
                    provide: SystemInfoService,
                    useValue: fakedSystemInfoService,
                },
                {
                    provide: UserPermissionService,
                    useValue: fakedUserPermissionService,
                },
                {
                    provide: AppConfigService,
                    useValue: { featureEnabled: () => false },
                },
            ],
        }).compileComponents();
    });

    beforeEach(async () => {
        fixtureRepo = TestBed.createComponent(RepositoryGridviewComponent);
        compRepo = fixtureRepo.componentInstance;
        compRepo.projectId = 1;
        compRepo.mode = '';
        compRepo.hasProjectAdminRole = true;
        compRepo.isCardView = false;
        fixtureRepo.detectChanges();
    });
    let originalTimeout;
    beforeEach(function () {
        originalTimeout = jasmine.DEFAULT_TIMEOUT_INTERVAL;
        jasmine.DEFAULT_TIMEOUT_INTERVAL = 100000;
    });
    afterEach(function () {
        jasmine.DEFAULT_TIMEOUT_INTERVAL = originalTimeout;
    });

    it('should create', () => {
        expect(compRepo).toBeTruthy();
    });
    it('should be card view', async () => {
        const cardViewButton =
            fixtureRepo.nativeElement.querySelector('.card-btn');
        cardViewButton.click();
        cardViewButton.dispatchEvent(new Event('click'));
        fixtureRepo.detectChanges();
        await fixtureRepo.whenStable();
        const cordView =
            fixtureRepo.nativeElement.querySelector('hbr-gridview');
        expect(cordView).toBeTruthy();
    });
    it('drops a typed term that a URL filter overtook', fakeAsync(() => {
        const spy = spyOn(
            fakedRepositoryService,
            'listRepositoriesResponse'
        ).and.callThrough();
        compRepo.filterComponent.filterTerms.next('old');
        queryParamMap.next(convertToParamMap({ name: 'redis' }));
        tick(500);
        expect(compRepo.lastFilteredRepoName).toBe('redis');
        expect(compRepo.lastFilteredRepoName).not.toBe('old');
        expect(spy).not.toHaveBeenCalledWith(
            jasmine.objectContaining({ q: encodeURIComponent('name=~old') })
        );
        compRepo.filterComponent.filterTerms.next('nginx');
        tick(500);
        expect(compRepo.lastFilteredRepoName).toBe('nginx');
        discardPeriodicTasks();
    }));
    it('shows the applied filter again when a load drops a typed term', fakeAsync(() => {
        compRepo.filterComponent.currentValue = 'ngi';
        compRepo.filterComponent.filterTerms.next('ngi');
        compRepo.clrLoad({ page: { from: 0, to: 14, size: 15 } });
        tick(500);
        expect(compRepo.filterComponent.currentValue).toBe('');
        expect(compRepo.lastFilteredRepoName).not.toBe('ngi');
        discardPeriodicTasks();
    }));
    it('should be list view', async () => {
        const listViewButton =
            fixtureRepo.nativeElement.querySelector('.list-btn');
        listViewButton.click();
        listViewButton.dispatchEvent(new Event('click'));
        fixtureRepo.detectChanges();
        await fixtureRepo.whenStable();
        const listView =
            fixtureRepo.nativeElement.querySelector('clr-datagrid');
        expect(listView).toBeTruthy();
    });
});
