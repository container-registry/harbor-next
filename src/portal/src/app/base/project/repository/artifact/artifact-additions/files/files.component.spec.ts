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
import { AdditionsService } from '../additions.service';
import { of } from 'rxjs';
import { ArtifactFilesComponent } from './files.component';
import { AdditionLink } from '../../../../../../../../ng-swagger-gen/models/addition-link';
import { ErrorHandler } from '../../../../../../shared/units/error-handler';
import { SharedTestingModule } from '../../../../../../shared/shared.module';
import { FilesItem } from 'src/app/shared/services/interface';

describe('FilesComponent', () => {
    let component: ArtifactFilesComponent;
    let fixture: ComponentFixture<ArtifactFilesComponent>;
    const mockedLink: AdditionLink = {
        absolute: false,
        href: '/test',
    };
    const filesList: FilesItem[] = [
        {
            path: 'src/index.js',
            size: 2300,
        },
        {
            path: 'README.md',
            size: 5632,
        },
        {
            path: 'foo/bar/1.txt',
            size: 2048,
        },
        {
            path: '.npmignore',
            size: 12,
        },
    ];

    const fakedAdditionsService = {
        getDetailByLink() {
            return of(filesList);
        },
    };
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [ArtifactFilesComponent],
            providers: [
                ErrorHandler,
                { provide: AdditionsService, useValue: fakedAdditionsService },
            ],
            schemas: [NO_ERRORS_SCHEMA],
        }).compileComponents();
    });

    beforeEach(() => {
        fixture = TestBed.createComponent(ArtifactFilesComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });
    it('should get files and render explorer', async () => {
        component.filesLink = mockedLink;
        component.ngOnInit();
        fixture.detectChanges();
        await fixture.whenStable();
        const tables = fixture.nativeElement.getElementsByTagName('table');
        expect(tables.length).toEqual(1);
        expect(component.totalFiles).toEqual(4);
        expect(component.totalFolders).toEqual(3);
        expect(component.visibleRows.map(row => row.path)).toEqual([
            'foo',
            'src',
            '.npmignore',
            'README.md',
        ]);
    });

    it('should expand folders', () => {
        component.filesList = filesList;
        component.refreshExplorer();

        component.toggleNode(component.visibleRows[0]);

        expect(component.visibleRows.map(row => row.path)).toEqual([
            'foo',
            'foo/bar',
            'foo/bar/1.txt',
            'src',
            '.npmignore',
            'README.md',
        ]);
    });

    it('should use unpacked size when present', () => {
        component.unpackedSize = 12345;
        component.filesList = filesList;

        expect(component.totalSize).toEqual(12345);
    });

    it('should filter files by type', () => {
        component.filesList = filesList;
        component.refreshExplorer();

        component.onTypeChange('js');

        expect(component.visibleRows.map(row => row.path)).toEqual([
            'src',
            'src/index.js',
        ]);
    });

    it('should show hidden files by default', () => {
        component.filesList = filesList;
        component.refreshExplorer();

        expect(component.visibleRows.map(row => row.path)).toContain(
            '.npmignore'
        );
    });
    it('should get files when link changes after init', async () => {
        component.filesLink = null;
        component.filesList = [];
        component.ngOnInit();

        component.filesLink = mockedLink;
        component.ngOnChanges();
        fixture.detectChanges();
        await fixture.whenStable();

        const tables = fixture.nativeElement.getElementsByTagName('table');
        expect(tables.length).toEqual(1);
        expect(component.isMaven).toBeFalse();
    });
    it('should render maven columnar files', async () => {
        const mavenFiles = [
            {
                filename: 'widget2-1.0.jar',
                extension: 'jar',
                digest: 'sha256:abc',
                size: 2048,
            },
            {
                filename: 'widget2-1.0.pom',
                extension: 'pom',
                digest: 'sha256:def',
                size: 512,
            },
        ];
        spyOn(component['additionsService'], 'getDetailByLink').and.returnValue(
            of(mavenFiles)
        );
        component.filesLink = mockedLink;
        component.ngOnInit();
        fixture.detectChanges();
        await fixture.whenStable();
        expect(component.isMaven).toBeTrue();
        expect(component.mavenFiles.length).toEqual(2);
        expect(component.mavenFileType(mavenFiles[0])).toEqual('RELEASE');
    });
});
