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
import { Component, OnInit, OnDestroy } from '@angular/core';
import { Subscription } from 'rxjs';
import { SessionService } from '../../../../shared/services/session.service';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';
import { StatisticHandler } from './statistic-handler.service';
import { Statistic } from '../../../../../../ng-swagger-gen/models/statistic';
import { StatisticService } from '../../../../../../ng-swagger-gen/services/statistic.service';
import { getSizeNumber, getSizeUnit } from '../../../../shared/units/utils';
import { SystemQuota } from '../../../../../../ng-swagger-gen/models/system-quota';
import { SystemquotaService } from '../../../../../../ng-swagger-gen/services/systemquota.service';
import {
    QUOTA_DANGER_COEFFICIENT,
    QUOTA_WARNING_COEFFICIENT,
} from '../../../../shared/entities/shared.const';

@Component({
    selector: 'statistics-panel',
    templateUrl: 'statistics-panel.component.html',
    styleUrls: ['statistics-panel.component.scss'],
})
export class StatisticsPanelComponent implements OnInit, OnDestroy {
    originalCopy: Statistic;
    // undefined until loaded, null when no global quota is set (404)
    systemQuota: SystemQuota | null;
    refreshSub: Subscription;
    constructor(
        private statistics: StatisticService,
        private systemQuotaService: SystemquotaService,
        private msgHandler: MessageHandlerService,
        private session: SessionService,
        private statisticHandler: StatisticHandler
    ) {}

    ngOnInit(): void {
        // Refresh
        this.refreshSub = this.statisticHandler.refreshChan$.subscribe(
            clear => {
                this.getStatistics();
            }
        );

        if (this.session.getCurrentUser()) {
            this.getStatistics();
        }
    }

    ngOnDestroy() {
        if (this.refreshSub) {
            this.refreshSub.unsubscribe();
        }
    }
    getStatistics(): void {
        this.statistics.getStatistic().subscribe(
            statistics => (this.originalCopy = statistics),
            error => {
                this.msgHandler.handleError(error);
            }
        );
        if (this.isValidSession) {
            this.getSystemQuota();
        }
    }
    getSystemQuota(): void {
        this.systemQuotaService.getSystemQuota().subscribe(
            quota => (this.systemQuota = quota),
            error => {
                // 404 means no global quota is configured, which is the default state
                this.systemQuota = null;
                if (!error || error.status !== 404) {
                    this.msgHandler.handleError(error);
                }
            }
        );
    }
    get hasSystemQuota(): boolean {
        return !!(this.systemQuota && this.systemQuota.hard);
    }
    get systemQuotaUsed(): number {
        return this.hasSystemQuota ? this.systemQuota.used.storage : 0;
    }
    get systemQuotaHard(): number {
        return this.hasSystemQuota ? this.systemQuota.hard.storage : 0;
    }
    get systemQuotaRatio(): number {
        return this.systemQuotaHard > 0
            ? this.systemQuotaUsed / this.systemQuotaHard
            : 0;
    }
    get isSystemQuotaDanger(): boolean {
        return this.systemQuotaRatio >= QUOTA_DANGER_COEFFICIENT;
    }
    get isSystemQuotaWarning(): boolean {
        return (
            this.systemQuotaRatio >= QUOTA_WARNING_COEFFICIENT &&
            this.systemQuotaRatio < QUOTA_DANGER_COEFFICIENT
        );
    }
    getHardSizeNumber(): number | string {
        return getSizeNumber(this.systemQuotaHard);
    }
    getHardSizeUnit(): number | string {
        return getSizeUnit(this.systemQuotaHard);
    }
    get isValidSession(): boolean {
        let user = this.session.getCurrentUser();
        return user && user.has_admin_role;
    }
    getSizeNumber(): number | string {
        // the global quota carries the effective usage (measured when available), the statistics only the accounted one
        if (this.hasSystemQuota) {
            return getSizeNumber(this.systemQuotaUsed);
        }
        if (this.originalCopy) {
            return getSizeNumber(this.originalCopy.total_storage_consumption);
        }
        return 0;
    }
    getSizeUnit(): number | string {
        if (this.hasSystemQuota) {
            return getSizeUnit(this.systemQuotaUsed);
        }
        if (this.originalCopy) {
            return getSizeUnit(this.originalCopy.total_storage_consumption);
        }
        return null;
    }
}
