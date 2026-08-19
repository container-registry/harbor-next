Feature: Federated Identity Provider with Kubernetes Service Account Tokens

  Background:
    Given a running Harbor
    And the KIND cluster is running

  # ─────────────────────────────────────────────
  # K8s SA token authentication
  # ─────────────────────────────────────────────

  @smoke @fedidp @k8s
  Scenario: Kubelet can pull images using K8s SA tokens via credential provider
    Given a fresh private project "k8s-pull"
    And a federated identity provider created from the cluster OIDC config named "k8s-idp"
    And IdP-level claim "aud" equals the Harbor registry host on "k8s-idp"
    And IdP-level claim "iss" equals the cluster issuer on "k8s-idp"
    And a federated robot "k8s-robot" linked to "k8s-idp" with pull permission on "k8s-pull"
    And robot-level claim "sub" equals "system:serviceaccount:default:default" on "k8s-robot" for "k8s-idp"
    When a test pod is deployed pulling the image from "k8s-pull"
    Then the pod starts running within 120 seconds

  @fedidp @k8s
  Scenario: SA token for an unlisted service account is rejected
    Given a fresh private project "k8s-reject"
    And a federated identity provider created from the cluster OIDC config named "reject-idp"
    And IdP-level claim "aud" equals the Harbor registry host on "reject-idp"
    And IdP-level claim "iss" equals the cluster issuer on "reject-idp"
    And a federated robot "reject-robot" linked to "reject-idp" with pull permission on "k8s-reject"
    And robot-level claim "sub" equals "system:serviceaccount:default:default" on "reject-robot" for "reject-idp"
    When a K8s SA token is issued for service account "other-sa" in namespace "default"
    And the token is used to request the Harbor API
    Then the request is unauthorized

  @fedidp @k8s
  Scenario: SA token authenticates correctly against the Harbor API
    Given a fresh private project "k8s-api"
    And a federated identity provider created from the cluster OIDC config named "api-idp"
    And IdP-level claim "aud" equals the Harbor registry host on "api-idp"
    And IdP-level claim "iss" equals the cluster issuer on "api-idp"
    And a federated robot "api-robot" linked to "api-idp" with pull permission on "k8s-api"
    And robot-level claim "sub" equals "system:serviceaccount:default:default" on "api-robot" for "api-idp"
    When a K8s SA token is issued for service account "default" in namespace "default"
    And the token is used to request the Harbor API
    Then the request is authorized
