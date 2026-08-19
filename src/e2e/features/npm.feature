Feature: npm package publish and install

  Background:
    Given a running Harbor

  @npm
  Scenario: A user publishes and installs an npm package from Harbor
    Given an npm package "harbor-e2e-npm" version "1.0.0"
    When the user runs npm publish to Harbor project "library"
    Then npm publish succeeds
    When the user runs npm install for that package from Harbor project "library"
    Then npm install succeeds
    And the installed npm package contains the published module
