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
import { StatisticsPanelComponent } from './statistics-panel.component';
import { CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { of } from 'rxjs';
import { SessionService } from '../../../../shared/services/session.service';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { StatisticHandler } from './statistic-handler.service';
import { AppConfigService } from '../../../../services/app-config.service';
import { Statistic } from '../../../../../../ng-swagger-gen/models/statistic';
import { SharedTestingModule } from '../../../../shared/shared.module';
import { StatisticService } from '../../../../../../ng-swagger-gen/services/statistic.service';
import { SystemquotaService } from '../../../../../../ng-swagger-gen/services/systemquota.service';
import { SystemQuota } from '../../../../../../ng-swagger-gen/models/system-quota';

describe('StatisticsPanelComponent', () => {
    const mockedStatistic: Statistic = {
        private_project_count: 2,
        private_repo_count: 0,
        public_project_count: 3,
        public_repo_count: 1,
        total_project_count: 5,
        total_repo_count: 1,
        total_storage_consumption: 4564,
    };
    let component: StatisticsPanelComponent;
    let fixture: ComponentFixture<StatisticsPanelComponent>;
    const mockStatisticsService = {
        getStatistic: () => of(mockedStatistic),
    };
    const mockedSystemQuota: SystemQuota = {
        hard: { storage: 10000 },
        used: { storage: 9500 },
        free: 500,
        used_source: 'accounted',
        allocated: 0,
        unlimited_projects: 0,
        enforce: true,
    };
    const mockSystemQuotaService = {
        getSystemQuota: () => of(mockedSystemQuota),
    };
    const mockSessionService = {
        getCurrentUser: () => {
            return {
                has_admin_role: true,
            };
        },
    };
    const mockAppConfigService = {
        getConfig: () => {
            return {
                registry_storage_provider_name: '',
            };
        },
    };
    const mockMessageHandlerService = {
        handleError: () => {},
    };
    const mockStatisticHandler = {
        refreshChan$: of(null),
    };
    beforeEach(() => {
        TestBed.configureTestingModule({
            schemas: [CUSTOM_ELEMENTS_SCHEMA],
            imports: [SharedTestingModule],
            declarations: [StatisticsPanelComponent],
            providers: [
                { provide: SessionService, useValue: mockSessionService },
                { provide: AppConfigService, useValue: mockAppConfigService },
                { provide: StatisticService, useValue: mockStatisticsService },
                {
                    provide: SystemquotaService,
                    useValue: mockSystemQuotaService,
                },
                { provide: StatisticHandler, useValue: mockStatisticHandler },
                {
                    provide: MessageHandlerService,
                    useValue: mockMessageHandlerService,
                },
            ],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(StatisticsPanelComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });
    it('should have 3 cards', async () => {
        fixture.detectChanges();
        await fixture.whenStable();
        const cards = fixture.nativeElement.querySelectorAll('.card');
        expect(cards.length).toEqual(3);
    });
    it('should display the global quota usage and limit when a quota is set', async () => {
        fixture.detectChanges();
        await fixture.whenStable();
        const sizeHtml: HTMLSpanElement =
            fixture.nativeElement.querySelector('.size-number');
        // 9500 bytes used of 10000 bytes, from the global quota rather than the statistics
        expect(sizeHtml.innerText).toEqual('9.28');
        const bar = fixture.nativeElement.querySelector('.system-quota-bar');
        expect(bar).toBeTruthy();
        expect(bar.querySelector('.progress.danger')).toBeTruthy();
    });
    it('should fall back to the statistics number without a global quota', async () => {
        component.systemQuota = null;
        fixture.detectChanges();
        await fixture.whenStable();
        const sizeHtml: HTMLSpanElement =
            fixture.nativeElement.querySelector('.size-number');
        expect(sizeHtml.innerText).toEqual('4.46');
        expect(
            fixture.nativeElement.querySelector('.system-quota-bar')
        ).toBeNull();
    });
});
