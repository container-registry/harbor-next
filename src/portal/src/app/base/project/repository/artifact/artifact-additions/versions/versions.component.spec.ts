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
import { of } from 'rxjs';
import { ArtifactVersionsComponent } from './versions.component';
import { AdditionsService } from '../additions.service';
import { AdditionLink } from '../../../../../../../../ng-swagger-gen/models/addition-link';
import { ErrorHandler } from '../../../../../../shared/units/error-handler';
import { SharedTestingModule } from '../../../../../../shared/shared.module';

describe('ArtifactVersionsComponent', () => {
    let component: ArtifactVersionsComponent;
    let fixture: ComponentFixture<ArtifactVersionsComponent>;
    const mockedLink: AdditionLink = {
        absolute: false,
        href: '/test',
    };
    const mockedPackument = {
        versions: [
            {
                version: '1.0.0',
                created: '2026-01-01T00:00:00Z',
                yanked: false,
                size: 2048,
            },
            {
                version: '1.1.0',
                created: '2026-02-01T00:00:00Z',
                yanked: true,
                size: 4096,
            },
        ],
        distTags: { latest: '1.1.0' },
    };

    const fakedAdditionsService = {
        getDetailByLink() {
            return of(mockedPackument);
        },
    };

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [ArtifactVersionsComponent],
            providers: [
                ErrorHandler,
                { provide: AdditionsService, useValue: fakedAdditionsService },
            ],
            schemas: [NO_ERRORS_SCHEMA],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(ArtifactVersionsComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should parse versions and dist-tags', async () => {
        component.versionsLink = mockedLink;
        component.ngOnInit();
        fixture.detectChanges();
        await fixture.whenStable();
        expect(component.versions.length).toEqual(2);
        expect(component.distTags.length).toEqual(1);
        expect(component.distTags[0].tag).toEqual('latest');
        expect(component.distTags[0].version).toEqual('1.1.0');
    });
});
