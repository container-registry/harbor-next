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
import { Component, OnInit } from '@angular/core';
import { concat } from 'rxjs';
import { finalize, toArray } from 'rxjs/operators';
import { ConfigureService } from 'ng-swagger-gen/services/configure.service';
import { Feature } from 'ng-swagger-gen/models/feature';
import { FeatureSection } from 'ng-swagger-gen/models/feature-section';
import { AppConfigService } from '../../../../services/app-config.service';
import { MessageHandlerService } from '../../../../shared/services/message-handler.service';

/**
 * Renders every feature toggle the backend registered with
 * src/pkg/feature, grouped into the sections it reports.
 */
@Component({
    selector: 'config-features',
    templateUrl: './features.component.html',
    styleUrls: ['./features.component.scss'],
})
export class FeaturesComponent implements OnInit {
    sections: FeatureSection[] = [];
    features: Feature[] = [];
    private pending: { [key: string]: boolean } = {};
    private loading = false;
    private onGoing = false;

    constructor(
        private configureService: ConfigureService,
        private appConfigService: AppConfigService,
        private messageHandler: MessageHandlerService
    ) {}

    // Covers the reload after a save too, so nothing can be edited or
    // resubmitted while the list is being replaced.
    get inProgress(): boolean {
        return this.loading || this.onGoing;
    }

    ngOnInit(): void {
        this.load();
    }

    load(): void {
        this.loading = true;
        this.configureService
            .listFeatures()
            .pipe(finalize(() => (this.loading = false)))
            .subscribe({
                next: list => {
                    this.sections = list?.sections || [];
                    this.features = list?.features || [];
                    this.pending = {};
                },
                error: error => this.messageHandler.handleError(error),
            });
    }

    featuresOf(section: FeatureSection): Feature[] {
        return this.features.filter(feature => feature.section === section.id);
    }

    value(feature: Feature): boolean {
        return feature.key in this.pending
            ? this.pending[feature.key]
            : feature.enabled;
    }

    disabled(feature: Feature): boolean {
        return !feature.editable || this.inProgress;
    }

    setEnabled(feature: Feature, enabled: boolean): void {
        if (enabled === feature.enabled) {
            delete this.pending[feature.key];
        } else {
            this.pending[feature.key] = enabled;
        }
    }

    hasChanges(): boolean {
        return Object.keys(this.pending).length > 0;
    }

    save(): void {
        const keys = Object.keys(this.pending);
        if (!keys.length) {
            return;
        }
        this.onGoing = true;
        // Sequential, so a rejected change (env lock, disable guard) stops
        // the rest and the reload shows what was actually stored.
        concat(
            ...keys.map(key =>
                this.configureService.updateFeature({
                    featureKey: key,
                    feature: { enabled: this.pending[key] },
                })
            )
        )
            .pipe(
                toArray(),
                finalize(() => (this.onGoing = false))
            )
            .subscribe({
                next: () => {
                    this.messageHandler.showSuccess('CONFIG.SAVE_SUCCESS');
                    this.refresh();
                },
                error: error => {
                    this.messageHandler.handleError(error);
                    this.refresh();
                },
            });
    }

    cancel(): void {
        this.pending = {};
    }

    private refresh(): void {
        this.load();
        // Other pages gate their UI on systeminfo features.
        this.appConfigService.load().subscribe();
    }
}
