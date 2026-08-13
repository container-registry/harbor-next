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
import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, throwError as observableThrowError } from 'rxjs';
import {
    buildHttpRequestOptions,
    HTTP_JSON_OPTIONS,
    HTTP_GET_OPTIONS,
    CURRENT_BASE_HREF,
} from '../units/utils';
import { RequestQueryParams } from './index';
import { catchError, map } from 'rxjs/operators';
import { FederatedIdp } from 'ng-swagger-gen/models';

/**
 * Define the service methods to handle Federated Identity Provider (IdP) related operations.
 *
 * Abstract base — concrete implementation {@link FederatedIdpDefaultService} is
 * bound via the `SharedModule` providers. Angular DI cannot instantiate an
 * abstract class, so `providedIn: 'root'` is intentionally not set here.
 *
 * @abstract
 * class FederatedIdpService
 */
export abstract class FederatedIdpService {
    /**
     * Get all federated identity providers.
     * Optionally filter by name or query parameters.
     *
     * @abstract
     * @param {string} [idpName]
     * @param {RequestQueryParams} [queryParams]
     * @returns {Observable<FederatedIdp[]>}
     */
    abstract getFederatedIdps(
        idpName?: string,
        queryParams?: RequestQueryParams
    ): Observable<FederatedIdp[]>;

    /**
     * Get a specific federated identity provider by ID.
     *
     * @abstract
     * @param {number | string} idpId
     * @returns {Observable<FederatedIdp>}
     */
    abstract getFederatedIdp(idpId: number | string): Observable<FederatedIdp>;

    /**
     * Create a new federated identity provider.
     *
     * @abstract
     * @param {FederatedIdp} idp
     * @returns {Observable<any>}
     */
    abstract createFederatedIdp(idp: FederatedIdp): Observable<any>;

    /**
     * Update an existing federated identity provider by ID.
     *
     * @abstract
     * @param {number | string} idpId
     * @param {FederatedIdp} idp
     * @returns {Observable<any>}
     */
    abstract updateFederatedIdp(
        idpId: number | string,
        idp: FederatedIdp
    ): Observable<any>;

    /**
     * Delete a federated identity provider by ID.
     *
     * @abstract
     * @param {number | string} idpId
     * @returns {Observable<any>}
     */
    abstract deleteFederatedIdp(idpId: number | string): Observable<any>;

    /**
     * Test connectivity or configuration of a federated identity provider.
     *
     * @abstract
     * @param {FederatedIdp} idp
     * @returns {Observable<any>}
     */
    abstract pingFederatedIdp(idp: FederatedIdp): Observable<any>;

    /**
     * Validate if the federated identity provider is associated with other entities (e.g., users, policies).
     *
     * @abstract
     * @param {number | string} idpId
     * @returns {Observable<any>}
     */
    abstract getFederatedIdpWithAssociations(
        idpId: number | string
    ): Observable<any>;

    /**
     * Get display-friendly name/label or description for a specific IdP type.
     *
     * @abstract
     * @param {string} type
     * @returns {string}
     */
    abstract getIdpTypeLabel(type: string): string;

    /**
     * Fetch OIDC configuration directly (replaces missing generated client)
     *
     * @abstract
     * @param {string} openidConfigUrl The OpenID Connect discovery document URL
     * @returns {Observable<any>}
     */
    abstract PingFederatedIdpOpenIDConfig(
        openidConfigUrl: string
    ): Observable<any>;

    /**
     * Fetch JWKS (JSON Web Key Set) from JWKS URI
     */
    abstract PingFederatedIdpJWKS(
        jwksUri: string
    ): Observable<{ [key: string]: any }>;
}

/**
 * Implement default service for FederatedIdp.
 *
 **
 * class FederatedIdpDefaultService
 * extends {FederatedIdpService}
 */
@Injectable()
export class FederatedIdpDefaultService extends FederatedIdpService {
    private _idpUrl: string;

    constructor(private http: HttpClient) {
        super();
        this._idpUrl = `${CURRENT_BASE_HREF}/federated-idps`;
    }

    public getFederatedIdps(
        idpName?: string,
        queryParams?: RequestQueryParams
    ): Observable<FederatedIdp[]> {
        if (!queryParams) {
            queryParams = new RequestQueryParams();
        }
        if (idpName) {
            queryParams = queryParams.set('name', idpName);
        }
        const requestUrl = this._idpUrl;
        return this.http
            .get(requestUrl, buildHttpRequestOptions(queryParams))
            .pipe(
                map(response => response as FederatedIdp[]),
                catchError(error => observableThrowError(error))
            );
    }

    public getFederatedIdp(idpId: number | string): Observable<FederatedIdp> {
        if (!idpId || +idpId <= 0) {
            return observableThrowError('Bad request argument.');
        }
        const requestUrl = `${this._idpUrl}/${idpId}`;
        return this.http.get(requestUrl, HTTP_GET_OPTIONS).pipe(
            map(response => response as FederatedIdp),
            catchError(error => observableThrowError(error))
        );
    }

    public createFederatedIdp(idp: FederatedIdp): Observable<any> {
        if (!idp) {
            return observableThrowError('Invalid Federated IDP.');
        }
        const requestUrl = this._idpUrl;
        return this.http
            .post<any>(requestUrl, JSON.stringify(idp), HTTP_JSON_OPTIONS)
            .pipe(catchError(error => observableThrowError(error)));
    }

    public updateFederatedIdp(
        idpId: number | string,
        idp: FederatedIdp
    ): Observable<any> {
        if (!idpId || +idpId <= 0) {
            return observableThrowError('Bad request argument.');
        }
        if (!idp) {
            return observableThrowError('Invalid Federated IDP.');
        }
        const requestUrl = `${this._idpUrl}/${idpId}`;
        return this.http
            .put<any>(requestUrl, JSON.stringify(idp), HTTP_JSON_OPTIONS)
            .pipe(catchError(error => observableThrowError(error)));
    }

    public deleteFederatedIdp(idpId: number | string): Observable<any> {
        if (!idpId || +idpId <= 0) {
            return observableThrowError('Bad request argument.');
        }
        const requestUrl = `${this._idpUrl}/${idpId}`;
        return this.http
            .delete<any>(requestUrl)
            .pipe(catchError(error => observableThrowError(error)));
    }

    public pingFederatedIdp(idp: FederatedIdp): Observable<any> {
        if (!idp) {
            return observableThrowError('Invalid Federated IDP.');
        }
        const requestUrl = `${this._idpUrl}/ping`;
        return this.http
            .post<any>(requestUrl, idp, HTTP_JSON_OPTIONS)
            .pipe(catchError(error => observableThrowError(error)));
    }

    public getFederatedIdpWithAssociations(
        idpId: number | string
    ): Observable<any> {
        if (!idpId || +idpId <= 0) {
            return observableThrowError('Bad request argument.');
        }
        const requestUrl = `${this._idpUrl}/${idpId}/associations`;
        return this.http
            .get<any>(requestUrl, HTTP_GET_OPTIONS)
            .pipe(catchError(error => observableThrowError(error)));
    }

    public getIdpTypeLabel(type: string): string {
        const IDP_TYPE_MAP: { [key: string]: string } = {
            oidc: 'OpenID Connect',
            saml: 'SAML',
            harbor: 'Harbor',
            ldap: 'LDAP',
        };
        return IDP_TYPE_MAP[type] || type;
    }

    public PingFederatedIdpOpenIDConfig(
        openidConfigUrl: string
    ): Observable<any> {
        if (!openidConfigUrl) {
            return observableThrowError('Invalid OpenID Configuration URL.');
        }

        const requestUrl = `${this._idpUrl}/openid-config`; // 👈 backend endpoint
        // const requestUrl = this._idpUrl;
        return this.http
            .post(
                requestUrl,
                { openid_config_url: openidConfigUrl },
                HTTP_JSON_OPTIONS
            )
            .pipe(
                map(response => response as FederatedIdp[]),
                catchError(error => observableThrowError(error))
            );

        // return this.http
        //     .post<{ [key: string]: any }>(
        //         requestUrl,
        //         { openid_config_url: openidConfigUrl }, // 👈 body format expected by backend
        //         HTTP_JSON_OPTIONS
        //     )
        //     .pipe(catchError(error => observableThrowError(error)));
    }

    public PingFederatedIdpJWKS(
        jwksUri: string
    ): Observable<{ [key: string]: any }> {
        if (!jwksUri) {
            return observableThrowError('Invalid JWKS URI.');
        }

        const requestUrl = `${this._idpUrl}/jwks`; // 👈 backend endpoint
        return this.http
            .post<{ [key: string]: any }>(
                requestUrl,
                { jwks: jwksUri }, // 👈 body format expected by backend
                HTTP_JSON_OPTIONS
            )
            .pipe(catchError(error => observableThrowError(error)));
    }
}
