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
import { PageNotFoundComponent } from './not-found.component';
import { CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { SharedTestingModule } from '../shared/shared.module';
import { Router } from '@angular/router';
import { PullUrlParserService } from '../shared/services/pull-url-parser.service';
import { ProjectService } from '../shared/services/project.service';
import { of, throwError } from 'rxjs';

describe('PageNotFoundComponent', () => {
    let component: PageNotFoundComponent;
    let fixture: ComponentFixture<PageNotFoundComponent>;
    let router: Router;
    let pullUrlParserService: PullUrlParserService;
    let projectService: ProjectService;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            schemas: [CUSTOM_ELEMENTS_SCHEMA],
            imports: [SharedTestingModule],
            declarations: [PageNotFoundComponent],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(PageNotFoundComponent);
        component = fixture.componentInstance;
        router = TestBed.inject(Router);
        pullUrlParserService = TestBed.inject(PullUrlParserService);
        projectService = TestBed.inject(ProjectService);
        spyOn(router, 'navigate');
    });

    afterEach(() => {
        component.ngOnDestroy();
    });

    it('should create', () => {
        spyOn(pullUrlParserService, 'parsePullUrl').and.returnValue(null);
        fixture.detectChanges();
        expect(component).toBeTruthy();
    });

    // ============================================================
    // CURRENT BEHAVIOR TESTS - For non-pull URL paths
    // ============================================================

    describe('Non-pull URL paths (404 behavior)', () => {
        beforeEach(() => {
            // Mock parser to return null (not a pull URL)
            spyOn(pullUrlParserService, 'parsePullUrl').and.returnValue(null);
        });

        it('should initialize leftSeconds to 5', () => {
            fixture.detectChanges();
            expect(component.leftSeconds).toBe(5);
        });

        it('should start countdown interval on init', () => {
            fixture.detectChanges();
            expect(component.timeInterval).toBeTruthy();
        });

        it('should decrement leftSeconds every second', fakeAsync(() => {
            fixture.detectChanges();
            expect(component.leftSeconds).toBe(5);

            tick(1000);
            expect(component.leftSeconds).toBe(4);

            tick(1000);
            expect(component.leftSeconds).toBe(3);

            // Complete the countdown to clear the timer
            tick(3000);
        }));

        it('should navigate to /harbor after 5 seconds', fakeAsync(() => {
            fixture.detectChanges();
            expect(router.navigate).not.toHaveBeenCalled();

            tick(5000);
            expect(router.navigate).toHaveBeenCalledWith(['harbor']);
        }));

        it('should clear interval on destroy', fakeAsync(() => {
            fixture.detectChanges();
            expect(component.timeInterval).toBeTruthy();

            component.ngOnDestroy();
            tick(5000);
        }));
    });

    // ============================================================
    // PULL URL ROUTING TESTS
    // ============================================================

    describe('Pull URL routing', () => {
        it('should redirect to project repository when pull URL is valid and user has access', fakeAsync(() => {
            // Mock parser to return parsed pull URL
            spyOn(pullUrlParserService, 'parsePullUrl').and.returnValue({
                projectName: 'myproject',
                repoName: 'nginx',
            });

            // Mock project service to return project
            spyOn(projectService, 'getProject').and.returnValue(
                of({ project_id: 123 } as any)
            );

            fixture.detectChanges();
            tick();

            expect(router.navigate).toHaveBeenCalledWith([
                '/harbor',
                'projects',
                123,
                'repositories',
                'nginx',
                'artifacts-tab',
            ]);
        }));

        it('should redirect for nested repository paths', fakeAsync(() => {
            spyOn(pullUrlParserService, 'parsePullUrl').and.returnValue({
                projectName: 'myproject',
                repoName: 'library/nginx',
            });

            spyOn(projectService, 'getProject').and.returnValue(
                of({ project_id: 456 } as any)
            );

            fixture.detectChanges();
            tick();

            expect(router.navigate).toHaveBeenCalledWith([
                '/harbor',
                'projects',
                456,
                'repositories',
                'library/nginx',
                'artifacts-tab',
            ]);
        }));

        it('should show 404 when project not found', fakeAsync(() => {
            spyOn(pullUrlParserService, 'parsePullUrl').and.returnValue({
                projectName: 'nonexistent',
                repoName: 'repo',
            });

            spyOn(projectService, 'getProject').and.returnValue(
                throwError(() => ({ status: 404 }))
            );

            fixture.detectChanges();
            tick(5000);

            // Should fallback to default 404 behavior
            expect(router.navigate).toHaveBeenCalledWith(['harbor']);
        }));

        it('should show 404 when user is not authorized (no project existence leak)', fakeAsync(() => {
            spyOn(pullUrlParserService, 'parsePullUrl').and.returnValue({
                projectName: 'private-project',
                repoName: 'repo',
            });

            spyOn(projectService, 'getProject').and.returnValue(
                throwError(() => ({ status: 403 }))
            );

            fixture.detectChanges();
            tick(5000);

            // Should show 404, not reveal project exists
            expect(router.navigate).toHaveBeenCalledWith(['harbor']);
        }));

        it('should show 404 when user is not authenticated', fakeAsync(() => {
            spyOn(pullUrlParserService, 'parsePullUrl').and.returnValue({
                projectName: 'private-project',
                repoName: 'repo',
            });

            spyOn(projectService, 'getProject').and.returnValue(
                throwError(() => ({ status: 401 }))
            );

            fixture.detectChanges();
            tick(5000);

            expect(router.navigate).toHaveBeenCalledWith(['harbor']);
        }));

        it('should show 404 for non-pull URL paths (excluded paths)', fakeAsync(() => {
            // Parser returns null for excluded paths like /harbor/*, /account/*
            spyOn(pullUrlParserService, 'parsePullUrl').and.returnValue(null);

            fixture.detectChanges();
            tick(5000);

            expect(router.navigate).toHaveBeenCalledWith(['harbor']);
        }));
    });
});
