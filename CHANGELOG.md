# Changelog

## [0.1.3](https://github.com/meigma/crossplane-cuefn/compare/v0.1.2...v0.1.3) (2026-07-01)


### Features

* add a verified shell install script ([#53](https://github.com/meigma/crossplane-cuefn/issues/53)) ([fcf8247](https://github.com/meigma/crossplane-cuefn/commit/fcf82472440811a55e4135c4b32f22c8808a9309))
* **nix:** add an in-repo flake for nix install ([#51](https://github.com/meigma/crossplane-cuefn/issues/51)) ([383d0fb](https://github.com/meigma/crossplane-cuefn/commit/383d0fb5677e0cdb6761aa98785874e52566551e))
* **release:** publish a Homebrew formula and Scoop manifest ([#49](https://github.com/meigma/crossplane-cuefn/issues/49)) ([b46bda5](https://github.com/meigma/crossplane-cuefn/commit/b46bda5056a6a7dda230b89b0f90bbf4e0fb215f))

## [0.1.2](https://github.com/meigma/crossplane-cuefn/compare/v0.1.1...v0.1.2) (2026-06-30)


### Bug Fixes

* collapse noisy CUE validation errors into one message ([#45](https://github.com/meigma/crossplane-cuefn/issues/45)) ([380dbd3](https://github.com/meigma/crossplane-cuefn/commit/380dbd3837f85fda14f6db0c0ebfbf18fa9bce66))
* **pkg,cli:** bind generated functionRefs to the installed Functions ([#44](https://github.com/meigma/crossplane-cuefn/issues/44)) ([500cf9d](https://github.com/meigma/crossplane-cuefn/commit/500cf9d74e53c920e606cace5b644606e42a96de))
* **render:** clearer error for a major-only module ref over OCI ([#43](https://github.com/meigma/crossplane-cuefn/issues/43)) ([b3bc034](https://github.com/meigma/crossplane-cuefn/commit/b3bc0346a5f771ec81d72e4f0290511d8861c519))
* **render:** fall back to a writable cache dir for the nonroot runtime ([#40](https://github.com/meigma/crossplane-cuefn/issues/40)) ([975e612](https://github.com/meigma/crossplane-cuefn/commit/975e612f87aa3d8c84337c16a44aaae07f3582f2))
* **schema:** emit XRD defaults for required, fully-defaultable fields ([#41](https://github.com/meigma/crossplane-cuefn/issues/41)) ([baa7d52](https://github.com/meigma/crossplane-cuefn/commit/baa7d52f1e2cded62d6d28b1034a4e9f5381a73c))

## [0.1.1](https://github.com/meigma/crossplane-cuefn/compare/v0.1.0...v0.1.1) (2026-06-30)


### Features

* **cli:** add cuefn render --required-resources ([#37](https://github.com/meigma/crossplane-cuefn/issues/37)) ([afc7196](https://github.com/meigma/crossplane-cuefn/commit/afc7196b104e9db3443d604618eeced57ce3888f))
* **function:** emit and receive required resources ([#36](https://github.com/meigma/crossplane-cuefn/issues/36)) ([ce381c4](https://github.com/meigma/crossplane-cuefn/commit/ce381c459c90b0a1723bcb249db3f2d8882c086f))
* **render:** support required resources in the engine ([#34](https://github.com/meigma/crossplane-cuefn/issues/34)) ([7625170](https://github.com/meigma/crossplane-cuefn/commit/7625170375f81963d25664c060abcdcb3afc233b))

## 0.1.0 (2026-06-30)


### ⚠ BREAKING CHANGES

* **render:** every module must nest input/resources/status under `out`. Pre-1.0, no migration shim.

### Features

* **cli:** wire dependency-aware local loading and add --cache-dir ([#15](https://github.com/meigma/crossplane-cuefn/issues/15)) ([3a70f63](https://github.com/meigma/crossplane-cuefn/commit/3a70f63f0e4da29bb517ea83bc953e22646eb72b))
* **example:** instantiate Kubernetes objects from cue.dev/x/k8s.io ([#17](https://github.com/meigma/crossplane-cuefn/issues/17)) ([6fe9932](https://github.com/meigma/crossplane-cuefn/commit/6fe9932cea6b0be17f9d75fc2a456ba16029edc3))
* **function:** add Crossplane composition function, cuefn render, and example render loop ([#6](https://github.com/meigma/crossplane-cuefn/issues/6)) ([6c36041](https://github.com/meigma/crossplane-cuefn/commit/6c36041ed801da3221a8317022944b45695d187e))
* **pkg:** build and push Crossplane Configuration xpkg via cuefn publish ([#8](https://github.com/meigma/crossplane-cuefn/issues/8)) ([fc3d388](https://github.com/meigma/crossplane-cuefn/commit/fc3d38804b22be5eb99d36a1d9e9a5c946ba1845))
* **release:** package and sign the cuefn Function xpkg ([#9](https://github.com/meigma/crossplane-cuefn/issues/9)) ([5000e29](https://github.com/meigma/crossplane-cuefn/commit/5000e29944f6bdad2b843fb4ef17d7227a9f2e5d))
* **render:** add OCI module loader with transitive deps, nonroot cache, and digest verification ([#5](https://github.com/meigma/crossplane-cuefn/issues/5)) ([7fa2199](https://github.com/meigma/crossplane-cuefn/commit/7fa2199571090f165daa209b47bd3b00422d6115))
* **render:** add offline CUE module render engine and module contract ([#4](https://github.com/meigma/crossplane-cuefn/issues/4)) ([b3a15d1](https://github.com/meigma/crossplane-cuefn/commit/b3a15d151d5ae4d8f52c094eaf67d84c0ebd87b8))
* **render:** make local module loading dependency-aware ([#14](https://github.com/meigma/crossplane-cuefn/issues/14)) ([75a3c4d](https://github.com/meigma/crossplane-cuefn/commit/75a3c4d8d11b639203c9d683c913d34b74fc4703))
* **render:** nest the module transform under a single `out` field ([#19](https://github.com/meigma/crossplane-cuefn/issues/19)) ([c825fe6](https://github.com/meigma/crossplane-cuefn/commit/c825fe64d35ca3c4383ea3815370ee5b086da755))
* **schema:** add CUE-to-XRD codegen with cuefn generate and validate ([#7](https://github.com/meigma/crossplane-cuefn/issues/7)) ([c76e1a8](https://github.com/meigma/crossplane-cuefn/commit/c76e1a80473f4cfa1a38f1c54883d92dfb4fa61e))

## Changelog

All notable changes to this project are documented here. This file is maintained
by [Release Please](https://github.com/googleapis/release-please) from
Conventional Commit history.
