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
import { Component, Input, OnChanges, OnInit, ViewChild } from '@angular/core';
import { InlineAlertComponent } from '../inline-alert/inline-alert.component';
import { AppConfigService } from '../../../services/app-config.service';
import { PushSection, pushSections } from './push-sections';

@Component({
    selector: 'hbr-push-image-button',
    templateUrl: './push-image.component.html',
    styleUrls: ['./push-image.scss'],

    providers: [],
})
export class PushImageButtonComponent implements OnChanges, OnInit {
    @Input() registryUrl: string = 'unknown';
    @Input() projectName: string = 'unknown';

    sections: PushSection[] = [];
    opened: boolean = false;
    activeId: string;

    @ViewChild('copyAlert') copyAlert: InlineAlertComponent;

    constructor(private appConfigService: AppConfigService) {}

    // Covers a button used without bound inputs, where ngOnChanges never runs.
    ngOnInit(): void {
        this.ngOnChanges();
    }

    ngOnChanges(): void {
        this.sections = pushSections(this.registryUrl, this.projectName, key =>
            this.appConfigService.featureEnabled(key)
        );
    }

    // Opens the dialog on the given section, or on the first one.
    open(id?: string): void {
        const section =
            this.sections.find(s => s.id === id) || this.sections[0];
        this.activeId = section?.id;
        if (this.copyAlert) {
            this.copyAlert.close();
        }
        this.opened = true;
    }

    onCpError($event: any): void {
        if (this.copyAlert) {
            this.copyAlert.showInlineError('PUSH_IMAGE.COPY_ERROR');
        }
    }
}
