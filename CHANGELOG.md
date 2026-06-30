# Changelog

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
