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
import { Component, Input, OnChanges, OnInit } from '@angular/core';
import { finalize } from 'rxjs/operators';
import { AdditionLink } from 'ng-swagger-gen/models';
import { AdditionsService } from '../additions.service';
import { ErrorHandler } from '../../../../../../shared/units/error-handler';
import { FilesItem } from 'src/app/shared/services/interface';
import { formatSize } from 'src/app/shared/units/utils';

interface FileExplorerNode {
    name: string;
    path: string;
    type: 'file' | 'directory';
    size: number;
    children: FileExplorerNode[];
    expanded: boolean;
    level: number;
}

// Maven GAV member file, emitted by the maven processor's FILES addition.
export interface MavenFileRef {
    filename: string;
    classifier?: string;
    extension: string;
    timestamp?: string;
    buildNumber?: number;
    digest: string;
    size: number;
}

@Component({
    selector: 'hbr-artifact-files',
    templateUrl: './files.component.html',
    styleUrls: ['./files.component.scss'],
})
export class ArtifactFilesComponent implements OnChanges, OnInit {
    @Input() filesLink: AdditionLink;
    @Input() unpackedSize: number = 0;
    @Input()
    set filesList(value: FilesItem[]) {
        if (value?.length) {
            this._filesList = value;
            this.refreshExplorer();
            return;
        }
        if (!this.filesLink) {
            this._filesList = [];
            this.refreshExplorer();
        }
    }
    get filesList(): FilesItem[] {
        return this._filesList;
    }
    mavenFiles: MavenFileRef[] = [];
    isMaven: boolean = false;
    loading: Boolean = false;
    fileTreeRows: FileExplorerNode[] = [];
    visibleRows: FileExplorerNode[] = [];
    fileTypes: string[] = [];
    selectedType: string = 'all';
    searchText: string = '';
    totalFiles: number = 0;
    totalFolders: number = 0;
    totalSize: number = 0;
    private _filesList: FilesItem[] = [];
    private fetchedHref: string = '';
    constructor(
        private errorHandler: ErrorHandler,
        private additionsService: AdditionsService
    ) {}

    ngOnInit(): void {
        this.loadFiles();
    }

    ngOnChanges(): void {
        this.loadFiles();
    }

    loadFiles() {
        if (!this.filesList?.length) {
            this.getFiles();
        }
    }
    getFiles() {
        if (this.filesLink && !this.filesLink.absolute && this.filesLink.href) {
            if (this.fetchedHref === this.filesLink.href) {
                return;
            }
            this.fetchedHref = this.filesLink.href;
            this.loading = true;
            this.additionsService
                .getDetailByLink(this.filesLink.href, false, false)
                .pipe(finalize(() => (this.loading = false)))
                .subscribe(
                    res => {
                        if (
                            Array.isArray(res) &&
                            res.length &&
                            res[0] &&
                            res[0].filename !== undefined
                        ) {
                            this.isMaven = true;
                            this.mavenFiles = res;
                        } else if (res && res.length) {
                            this.filesList = res;
                        }
                    },
                    error => {
                        this.fetchedHref = '';
                        this.errorHandler.error(error);
                    }
                );
        }
    }

    mavenFileType(file: MavenFileRef): string {
        return file.timestamp ? 'SNAPSHOT' : 'RELEASE';
    }

    shortDigest(digest: string): string {
        if (!digest) {
            return '';
        }
        return digest.replace(/^sha256:/, '');
    }

    getChildren(folder: any) {
        return folder.children || [];
    }

    isFlatFiles(): boolean {
        return this.filesList?.some(file => !!file.path);
    }

    refreshExplorer(): void {
        if (!this.isFlatFiles()) {
            this.fileTreeRows = [];
            this.visibleRows = [];
            return;
        }

        this.fileTreeRows = this.buildFileTree(this.filesList);
        this.totalFiles = this.filesList.length;
        this.totalFolders = this.countFolders(this.fileTreeRows);
        this.totalSize =
            this.unpackedSize || this.totalFileSize(this.filesList);
        this.fileTypes = this.buildFileTypes(this.filesList);
        this.applyFilters();
    }

    buildFileTree(files: FilesItem[]): FileExplorerNode[] {
        const root: FileExplorerNode = this.newNode('', '', 'directory', 0, 0);
        const directories = new Map<string, FileExplorerNode>();
        directories.set('', root);

        files.forEach(file => {
            const path = this.filePath(file).replace(/^\/+/, '');
            if (!path) {
                return;
            }
            const parts = path.split('/').filter(part => !!part);
            let parent = root;
            let currentPath = '';

            parts.forEach((part, index) => {
                currentPath = currentPath ? `${currentPath}/${part}` : part;
                const isFile = index === parts.length - 1;
                if (isFile) {
                    parent.children.push(
                        this.newNode(
                            part,
                            currentPath,
                            'file',
                            file.size || 0,
                            index
                        )
                    );
                    return;
                }

                let directory = directories.get(currentPath);
                if (!directory) {
                    directory = this.newNode(
                        part,
                        currentPath,
                        'directory',
                        0,
                        index
                    );
                    directories.set(currentPath, directory);
                    parent.children.push(directory);
                }
                parent = directory;
            });
        });

        this.sortNodes(root.children);
        return root.children;
    }

    newNode(
        name: string,
        path: string,
        type: 'file' | 'directory',
        size: number,
        level: number
    ): FileExplorerNode {
        return {
            name,
            path,
            type,
            size,
            children: [],
            expanded: type === 'directory' && level > 0,
            level,
        };
    }

    sortNodes(nodes: FileExplorerNode[]): void {
        nodes.sort((a, b) => {
            if (a.type !== b.type) {
                return a.type === 'directory' ? -1 : 1;
            }
            return a.name.localeCompare(b.name);
        });
        nodes.forEach(node => this.sortNodes(node.children));
    }

    applyFilters(): void {
        const rows: FileExplorerNode[] = [];
        this.fileTreeRows.forEach(node => this.collectVisibleNode(node, rows));
        this.visibleRows = rows;
    }

    collectVisibleNode(
        node: FileExplorerNode,
        rows: FileExplorerNode[]
    ): boolean {
        if (!this.nodeHasVisibleMatch(node)) {
            return false;
        }
        rows.push(node);
        if (
            node.type === 'directory' &&
            (node.expanded || this.hasActiveFilter())
        ) {
            node.children.forEach(child =>
                this.collectVisibleNode(child, rows)
            );
        }
        return true;
    }

    nodeHasVisibleMatch(node: FileExplorerNode): boolean {
        return (
            this.nodeMatches(node) ||
            node.children.some(child => this.nodeHasVisibleMatch(child))
        );
    }

    nodeMatches(node: FileExplorerNode): boolean {
        const search = this.searchText.trim().toLowerCase();
        if (search && !node.path.toLowerCase().includes(search)) {
            return false;
        }
        if (this.selectedType !== 'all' && node.type === 'file') {
            return this.fileType(node.path) === this.selectedType;
        }
        return this.selectedType === 'all';
    }

    hasActiveFilter(): boolean {
        return this.searchText.trim() !== '' || this.selectedType !== 'all';
    }

    countFolders(nodes: FileExplorerNode[]): number {
        return nodes.reduce((total, node) => {
            if (node.type !== 'directory') {
                return total;
            }
            return total + 1 + this.countFolders(node.children);
        }, 0);
    }

    buildFileTypes(files: FilesItem[]): string[] {
        const types = new Set<string>();
        files.forEach(file => types.add(this.fileType(this.filePath(file))));
        return Array.from(types).sort();
    }

    totalFileSize(files: FilesItem[]): number {
        return files.reduce((total, file) => total + (file.size || 0), 0);
    }

    fileType(path: string): string {
        const name = path.split('/').pop() || '';
        const index = name.lastIndexOf('.');
        if (index <= 0 || index === name.length - 1) {
            return 'none';
        }
        return name.slice(index + 1).toLowerCase();
    }

    toggleNode(node: FileExplorerNode): void {
        node.expanded = !node.expanded;
        this.applyFilters();
    }

    toggleDirectory(node: FileExplorerNode): void {
        if (node.type === 'directory') {
            this.toggleNode(node);
        }
    }

    onSearch(value: string): void {
        this.searchText = value;
        this.applyFilters();
    }

    onTypeChange(value: string): void {
        this.selectedType = value;
        this.applyFilters();
    }

    displayPath(node: FileExplorerNode): string {
        return '/' + node.path;
    }

    typeLabel(node: FileExplorerNode): string {
        return node.type === 'directory' ? 'Folder' : 'File';
    }

    rowIndent(node: FileExplorerNode): string {
        return `${12 + node.level * 18}px`;
    }

    folderCountLabel(): string {
        return `${this.totalFiles} files, ${this.totalFolders} folders`;
    }

    fileTypesLabel(): string {
        return this.fileTypes.length ? this.fileTypes.join(', ') : '-';
    }

    typeOptionLabel(type: string): string {
        return type === 'none' ? 'No extension' : type;
    }

    filePath(file: FilesItem): string {
        return file.path || file.name || '';
    }

    sizeTransform(tagSize: string): string {
        return formatSize(tagSize);
    }
}
