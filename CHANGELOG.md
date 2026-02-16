# Changelog

## v0.4.0
* Add option to pass configmap with mapping between SA and IAM role ([#142](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/139), @olemarkus)
* add cert manager deployment ([#139](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/139), @prasita123)
* Add warning for --in-cluster=true to README ([#138](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/138), @nckturner)
* Use certwatcher to support mounting cert-manager certificates. ([#134](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/134), @colinhoglund)

## v0.3.0
* Fix serviceaccount regional sts annotation not taking effect unless flag is true ([#120](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/120), @wongma7)
* add metrics for knowing adoption ([#122](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/122), @jyotimahapatra)
* Refactor service account cache to accept informer arg and unit test it  ([#117](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/117), @wongma7)
* Add additional log statements and update client-go ([#92](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/92), @shravan-achar)
* Add a debug handler to list cache contents and log mutation decision ([#90](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/90), @shravan-achar)
* README: add documentation for running containers as non-root ([#88](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/88), @josselin-c)
* patch pod spec even if it's already been patched ([#62](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/62), @yangkev)
* Fix panic in cache informer ([#70](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/70), @mfojtik)
* Change master branch image tag and update README ([#81](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/81), @josselin-c)
* Add github worflow to automate docker image creation ([#80](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/80), @josselin-c)
* deploy: add sideEffects to webhook ([#79](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/79), @sjenning)
* Add attribution document to container ([#76](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/76), @josselin-c)
* Update Makefile to delete created tls cert ([#60](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/60), @Smuggla)
* Update ecr login command for both aws-cli v1 and v2 ([#53](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/53), @int128)

## v0.2.0
* Making changes for finding the oidc discovery endpoint in README.md ([#19](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/19), @bhks)
* Fix jsonpatch operation if no volumes are present ([#18](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/18), @FaHeymann)
* Added K8s 1.16 compatible kid to jwks JSON ([#13](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/13), @micahhausler)
* Removed key id from jwks JSON hack script  ([#9](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/9), @micahhausler)
* Added self-hosted cluster setup guide ([#7](https://github.com/aws/amazon-eks-pod-identity-webhook/pull/7), @micahhausler)
