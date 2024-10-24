# https://github.com/beatcracker/toptout/blob/master/examples/toptout_bash.sh
set_telemetry_env () {
  if [[ ${2} ]]
  then
    export "${1}"="${2}"
  fi
}

# Canvas LMS
# https://github.com/instructure/canvas-lms

# Usage data | Official
# https://github.com/instructure/canvas-lms/blob/dc0e7b50e838fcca6f111082293b8faf415aff28/lib/tasks/db_load_data.rake#L154
set_telemetry_env 'CANVAS_LMS_STATS_COLLECTION' 'opt_out'

# Canvas LMS
# https://github.com/instructure/canvas-lms

# Usage data | Unofficial
# https://github.com/instructure/canvas-lms/blob/dc0e7b50e838fcca6f111082293b8faf415aff28/lib/tasks/db_load_data.rake#L16
set_telemetry_env 'TELEMETRY_OPT_IN' ''

# Eternal Terminal
# https://github.com/MisterTea/EternalTerminal

# Crash data
set_telemetry_env 'ET_NO_TELEMETRY' 'ANY_VALUE'

# Homebrew
# https://brew.sh

# Usage data
set_telemetry_env 'HOMEBREW_NO_ANALYTICS' '1'

# Homebrew
# https://brew.sh

# Usage data (alternate environment variable)
# https://github.com/Homebrew/brew/blob/6ad92949e910041416d84a53966ec46b873e069f/Library/Homebrew/utils/analytics.sh#L38
set_telemetry_env 'HOMEBREW_NO_ANALYTICS_THIS_RUN' '1'

# Homebrew
# https://brew.sh

# Update check
# https://docs.brew.sh/Manpage
set_telemetry_env 'HOMEBREW_NO_AUTO_UPDATE' '1'

# LYNX VFX
# https://github.com/LucaScheller/VFX-LYNX

# Usage data
set_telemetry_env 'LYNX_ANALYTICS' '0'

# Quickwit
# https://quickwit.io/

# Usage data
set_telemetry_env 'DISABLE_QUICKWIT_TELEMETRY' '1'

# Automagica
# https://automagica.com/

# Usage data
set_telemetry_env 'AUTOMAGICA_NO_TELEMETRY' 'ANY_VALUE'

# AWS SAM CLI
# https://aws.amazon.com/serverless/sam/

# Usage data
set_telemetry_env 'SAM_CLI_TELEMETRY' '0'

# Azure CLI
# https://docs.microsoft.com/en-us/cli/azure

# Usage data
set_telemetry_env 'AZURE_CORE_COLLECT_TELEMETRY' '0'

# Google Cloud SDK
# https://cloud.google.com/sdk

# Usage data
set_telemetry_env 'CLOUDSDK_CORE_DISABLE_USAGE_REPORTING' 'true'

# Hoockdeck CLI
# https://hookdeck.com/

# Usage data
# https://github.com/hookdeck/hookdeck-cli/blob/8c2e18bfd5d413e1d2418c5a73d56791b3bfb513/pkg/hookdeck/client.go#L56-L61
set_telemetry_env 'HOOKDECK_CLI_TELEMETRY_OPTOUT' 'ANY_VALUE'

# Netdata
# https://www.netdata.cloud

# Usage data
set_telemetry_env 'DO_NOT_TRACK' '1'

# Stripe CLI
# https://stripe.com/docs/stripe-cli

# Usage data
set_telemetry_env 'STRIPE_CLI_TELEMETRY_OPTOUT' '1'

# Tilt
# https://tilt.dev

# Usage data
set_telemetry_env 'DO_NOT_TRACK' '1'

# Mattermost Server
# https://mattermost.com/

# Diagnostic data
# https://docs.mattermost.com/manage/telemetry.html#error-and-diagnostics-reporting-feature
set_telemetry_env 'MM_LOGSETTINGS_ENABLEDIAGNOSTICS' 'false'

# Mattermost Server
# https://mattermost.com/

# Security Update Check
# https://docs.mattermost.com/manage/telemetry.html#security-update-check-feature
set_telemetry_env 'MM_SERVICESETTINGS_ENABLESECURITYFIXALERT' 'false'

# Feast
# https://feast.dev/

# Usage data
set_telemetry_env 'FEAST_TELEMETRY' 'False'

# InfluxDB
# https://www.influxdata.com/

# Usage data
# https://docs.influxdata.com/influxdb/v2.0/reference/config-options/
set_telemetry_env 'INFLUXD_REPORTING_DISABLED' 'true'

# Meltano
# https://www.meltano.com/

# Usage data
set_telemetry_env 'MELTANO_DISABLE_TRACKING' 'True'

# Quilt
# https://quiltdata.com/

# Usage data
set_telemetry_env 'QUILT_DISABLE_USAGE_METRICS' 'True'

# aliBuild
# https://github.com/alisw/alibuild

# Usage data
set_telemetry_env 'ALIBUILD_NO_ANALYTICS' '1'

# Angular
# https://angular.io

# Usage data
# https://angular.io/analytics
set_telemetry_env 'NG_CLI_ANALYTICS' 'false'

# Angular
# https://angular.io

# Usage data (custom)
# https://angular.io/cli/usage-analytics-gathering
set_telemetry_env 'NG_CLI_ANALYTICS_SHARE' 'false'

# Appc Daemon
# https://github.com/appcelerator/appc-daemon

# Usage data
set_telemetry_env 'APPCD_TELEMETRY' '0'

# App Center CLI
# https://github.com/microsoft/appcenter-cli/

# Usage data (env. var)
# https://github.com/microsoft/appcenter-cli/blob/master/src/util/profile/telemetry.ts
set_telemetry_env 'MOBILE_CENTER_TELEMETRY' 'off'

# Arduino CLI
# https://arduino.github.io/arduino-cli/latest/

# Internal metrics
set_telemetry_env 'ARDUINO_METRICS_ENABLED' 'false'

# Bot Framework CLI
# https://github.com/microsoft/botframework-cli

# Usage data
# https://github.com/microsoft/botframework-cli/tree/main/packages/cli#bf-configsettelemetry
set_telemetry_env 'BF_CLI_TELEMETRY' 'false'

# Carbon Design System
# https://www.carbondesignsystem.com/

# Usage data
set_telemetry_env 'CARBON_TELEMETRY_DISABLED' '1'

# choosenim
# https://github.com/dom96/choosenim

# Usage data
set_telemetry_env 'CHOOSENIM_NO_ANALYTICS' '1'

# CocoaPods
# https://cocoapods.org/

# Usage data
set_telemetry_env 'COCOAPODS_DISABLE_STATS' 'true'

# Apache Cordova CLI
# https://cordova.apache.org

# Usage data
set_telemetry_env 'CI' 'ANY_VALUE'

# Cube.js
# https://cube.dev/

# Usage data
# https://cube.dev/docs/reference/environment-variables#general
set_telemetry_env 'CUBEJS_TELEMETRY' 'false'

# Dagster
# https://dagster.io/

# Usage data (environment variable)
# https://github.com/dagster-io/dagster/blob/master/python_modules/dagit/dagit/telemetry.py
set_telemetry_env 'DAGSTER_DISABLE_TELEMETRY' 'ANY_VALUE'

# .NET Interactive
# https://github.com/dotnet/interactive

# Usage data
set_telemetry_env 'DOTNET_INTERACTIVE_CLI_TELEMETRY_OPTOUT' '1'

# dotnet-svcutil
# https://docs.microsoft.com/en-us/dotnet/core/additional-tools/dotnet-svcutil-guide

# Usage data
set_telemetry_env 'DOTNET_SVCUTIL_TELEMETRY_OPTOUT' '1'

# Fastlane
# https://fastlane.tools/

# Usage data
set_telemetry_env 'FASTLANE_OPT_OUT_USAGE' 'YES'

# Flagsmith API
# https://flagsmith.com/

# Usage data
set_telemetry_env 'TELEMETRY_DISABLED' 'ANY_VALUE'

# Gatsby
# https://www.gatsbyjs.org

# Usage data
set_telemetry_env 'GATSBY_TELEMETRY_DISABLED' '1'

# Golang
# https://go.dev/

# Usage data
# https://github.com/golang/go/discussions/58409
set_telemetry_env 'GOTELEMETRY' 'off'

# Hasura GraphQL engine
# https://hasura.io

# Usage data (CLI and Console)
set_telemetry_env 'HASURA_GRAPHQL_ENABLE_TELEMETRY' 'false'

# Humbug
# https://github.com/bugout-dev/humbug

# Usage data
# https://github.com/bugout-dev/humbug/issues/13
set_telemetry_env 'BUGGER_OFF' '1'

# ImageGear
# https://www.accusoft.com/products/imagegear-collection/imagegear/

# Usage data
# https://help.accusoft.com/ImageGear/v18.8/Linux/Installation.html
case "$OSTYPE" in
  linux*)
    set_telemetry_env 'IG_PRO_OPT_OUT' 'YES'
  ;;
esac

# MeiliSearch
# https://github.com/meilisearch/MeiliSearch

# Usage data and crash reports
set_telemetry_env 'MEILI_NO_ANALYTICS' 'true'

# ML.NET CLI
# https://docs.microsoft.com/en-us/dotnet/machine-learning/automate-training-with-cli

# Usage data
set_telemetry_env 'MLDOTNET_CLI_TELEMETRY_OPTOUT' 'True'

# mssql-cli
# https://github.com/dbcli/mssql-cli

# Usage data
set_telemetry_env 'MSSQL_CLI_TELEMETRY_OPTOUT' 'True'

# .NET Core SDK
# https://docs.microsoft.com/en-us/dotnet/core/tools/index

# Usage data
set_telemetry_env 'DOTNET_CLI_TELEMETRY_OPTOUT' 'true'

# Next.js
# https://nextjs.org

# Usage data
set_telemetry_env 'NEXT_TELEMETRY_DISABLED' '1'

# NocoDB
# https://www.nocodb.com/

# Usage data
set_telemetry_env 'NC_DISABLE_TELE' '1'

# Nuxt.js
# https://nuxtjs.org/

# Usage data
set_telemetry_env 'NUXT_TELEMETRY_DISABLED' '1'

# One Codex API - Python Client Library and CLI
# https://www.onecodex.com/

# Usage data
set_telemetry_env 'ONE_CODEX_NO_TELEMETRY' 'True'

# Ory
# https://www.ory.sh/

# Usage data
set_telemetry_env 'SQA_OPT_OUT' 'true'

# Oryx
# https://github.com/microsoft/Oryx

# Usage data
set_telemetry_env 'ORYX_DISABLE_TELEMETRY' 'true'

# otel-launcher-node
# https://github.com/lightstep/otel-launcher-node/

# Usage data
set_telemetry_env 'LS_METRICS_HOST_ENABLED' '0'

# Pants
# https://www.pantsbuild.org/

# Usage data
# https://www.pantsbuild.org/docs/reference-anonymous-telemetry
set_telemetry_env 'PANTS_ANONYMOUS_TELEMETRY_ENABLED' 'false'

# Prisma
# https://www.prisma.io/

# Usage data
# https://www.prisma.io/docs/concepts/more/telemetry#usage-data
set_telemetry_env 'CHECKPOINT_DISABLE' '1'

# projector-cli
# https://github.com/projector-cli/projector-cli

# Usage data
set_telemetry_env 'TELEMETRY_ENABLED' '0'

# PROSE Code Accelerator SDK
# https://www.microsoft.com/en-us/research/group/prose/

# Usage data
set_telemetry_env 'PROSE_TELEMETRY_OPTOUT' 'ANY_VALUE'

# Rasa
# https://rasa.com/

# Usage data
set_telemetry_env 'RASA_TELEMETRY_ENABLED' 'false'

# ReportPortal (JS client)
# https://github.com/reportportal/client-javascript

# Usage data
set_telemetry_env 'REPORTPORTAL_CLIENT_JS_NO_ANALYTICS' 'true'

# ReportPortal (Pytest plugin)
# https://github.com/reportportal/agent-python-pytest

# Usage data
set_telemetry_env 'AGENT_NO_ANALYTICS' '1'

# RESTler
# https://github.com/microsoft/restler-fuzzer

# Usage data
# https://github.com/microsoft/restler-fuzzer/blob/main/docs/user-guide/Telemetry.md
set_telemetry_env 'RESTLER_TELEMETRY_OPTOUT' '1'

# Rockset CLI
# https://rockset.com/

# Usage data
set_telemetry_env 'ROCKSET_CLI_TELEMETRY_OPTOUT' '1'

# Testim Root Cause
# https://github.com/testimio/root-cause

# Usage data
set_telemetry_env 'SUGGESTIONS_OPT_OUT' 'ANY_VALUE'

# Rover CLI
# https://www.apollographql.com/docs/rover/

# Usage data
set_telemetry_env 'APOLLO_TELEMETRY_DISABLED' '1'

# Salto CLI
# https://www.salto.io/

# Usage data
set_telemetry_env 'SALTO_TELEMETRY_DISABLE' '1'

# Serverless Framework
# https://www.serverless.com/

# Usage data
set_telemetry_env 'SLS_TELEMETRY_DISABLED' '1'

# Serverless Framework
# https://www.serverless.com/

# Usage data (alternate environment variable)
# https://github.com/serverless/serverless/blob/18d4d69eb3b1220814ab031690b6ef899280a93a/lib/utils/telemetry/are-disabled.js#L5-L9
set_telemetry_env 'SLS_TRACKING_DISABLED' '1'

# Salesforce CLI
# https://developer.salesforce.com/tools/sfdxcli

# Usage data
set_telemetry_env 'SFDX_DISABLE_TELEMETRY' 'true'

# Salesforce CLI
# https://developer.salesforce.com/tools/sfdxcli

# Usage data (alternate environment variable)
# https://github.com/forcedotcom/sfdx-core/blob/31fc950dd3fea9696d15e28ad944f07a08349e60/src/config/envVars.ts#L176-L179
set_telemetry_env 'SF_DISABLE_TELEMETRY' 'true'

# SKU
# https://github.com/seek-oss/sku

# Usage data
set_telemetry_env 'SKU_TELEMETRY' 'false'

# Strapi
# https://strapi.io/

# Usage data
# https://strapi.io/documentation/developer-docs/latest/setup-deployment-guides/configurations.html#environment
set_telemetry_env 'STRAPI_TELEMETRY_DISABLED' 'true'

# Strapi
# https://strapi.io/

# Update check
# https://strapi.io/documentation/developer-docs/latest/setup-deployment-guides/configurations.html#environment
set_telemetry_env 'STRAPI_DISABLE_UPDATE_NOTIFICATION' 'true'

# Tuist
# https://tuist.io/

# Usage data
set_telemetry_env 'TUIST_STATS_OPT_OUT' '1'

# TYPO3
# https://github.com/instructure/canvas-lms

# Update check
# https://docs.typo3.org/m/typo3/guide-installation/master/en-us/Legacy/Index.html#disabling-the-core-updater
set_telemetry_env 'TYPO3_DISABLE_CORE_UPDATER' '1'

# TYPO3
# https://github.com/instructure/canvas-lms

# Update check (Apache compatibility)
# https://forge.typo3.org/issues/53188
set_telemetry_env 'REDIRECT_TYPO3_DISABLE_CORE_UPDATER' '1'

# vstest
# https://github.com/microsoft/vstest/

# Usage data
# https://github.com/microsoft/vstest/blob/main/src/vstest.console/TestPlatformHelpers/TestRequestManager.cs#L1047
set_telemetry_env 'VSTEST_TELEMETRY_OPTEDIN' '0'

# VueDX
# https://github.com/znck/vue-developer-experience

# Usage data
set_telemetry_env 'VUEDX_TELEMETRY' 'off'

# webhint
# https://webhint.io/

# Usage data
set_telemetry_env 'HINT_TELEMETRY' 'off'

# Webiny
# https://www.webiny.com/

# Usage data (env. var)
# https://github.com/webiny/webiny-js/blob/0240c2000d1743160c601ae4ce40dd2f949d4d07/packages/telemetry/react.js#L9
set_telemetry_env 'REACT_APP_WEBINY_TELEMETRY' 'false'

# Yarn 2
# https://yarnpkg.com/

# Usage data
# https://yarnpkg.com/advanced/telemetry
set_telemetry_env 'YARN_ENABLE_TELEMETRY' '0'

# AutomatedLab
# https://github.com/AutomatedLab/AutomatedLab

# Usage data
set_telemetry_env 'AUTOMATEDLAB_TELEMETRY_OPTIN' '0'

# AutomatedLab
# https://github.com/AutomatedLab/AutomatedLab

# Usage data (legacy env. var.)
set_telemetry_env 'AUTOMATEDLAB_TELEMETRY_OPTOUT' '1'

# AutoSPInstaller Online
# https://github.com/IvanJosipovic/AutoSPInstallerOnline

# Usage data
# https://github.com/IvanJosipovic/AutoSPInstallerOnline/blob/3b4d0e3a7220632a00e36194ce540b8b34e9ed18/AutoSPInstaller.Core/Startup.cs#L36
set_telemetry_env 'DisableTelemetry' 'True'

# Batect
# https://batect.dev/

# Usage data
set_telemetry_env 'BATECT_ENABLE_TELEMETRY' 'false'

# Chef Workstation
# https://docs.chef.io/workstation/

# Usage data
# https://docs.chef.io/workstation/privacy/#opting-out
set_telemetry_env 'CHEF_TELEMETRY_OPT_OUT' '1'

# Consul
# https://www.consul.io/

# Update check
# https://www.consul.io/docs/agent/options#disable_update_check
set_telemetry_env 'CHECKPOINT_DISABLE' 'ANY_VALUE'

# Dagger
# https://dagger.io/

# Usage data
set_telemetry_env 'DO_NOT_TRACK' '1'

# decK
# https://github.com/Kong/deck

# Usage data
set_telemetry_env 'DECK_ANALYTICS' 'off'

# Earthly
# https://earthly.dev/

# Usage data
# https://github.com/earthly/earthly/blob/main/CHANGELOG.md#v0518---2021-07-08
set_telemetry_env 'EARTHLY_DISABLE_ANALYTICS' '1'

# F5 BIG-IP Terraform provider
# https://registry.terraform.io/providers/F5Networks/bigip/latest/docs

# Usage data
set_telemetry_env 'TEEM_DISABLE' 'true'

# F5 CLI
# https://clouddocs.f5.com/sdk/f5-cli/

# Usage data
set_telemetry_env 'F5_ALLOW_TELEMETRY' 'false'

# Infracost
# https://www.infracost.io/

# Usage data
# https://www.infracost.io/docs/integrations/environment_variables/#infracost_self_hosted_telemetry
set_telemetry_env 'INFRACOST_SELF_HOSTED_TELEMETRY' 'false'

# Infracost
# https://www.infracost.io/

# Update check
# https://www.infracost.io/docs/integrations/environment_variables/#infracost_skip_update_check
set_telemetry_env 'INFRACOST_SKIP_UPDATE_CHECK' 'true'

# Kics
# https://kics.io/

# Usage data (current)
# https://github.com/Checkmarx/kics/issues/3876
set_telemetry_env 'DISABLE_CRASH_REPORT' '1'

# Kics
# https://kics.io/

# Usage data (legacy)
# https://github.com/Checkmarx/kics/issues/3876
set_telemetry_env 'KICS_COLLECT_TELEMETRY' '0'

# kPow
# https://kpow.io/

# Usage data
# https://docs.kpow.io/about/data-collection#how-do-i-opt-out
set_telemetry_env 'ALLOW_UI_ANALYTICS' 'false'

# kubeapt
# https://github.com/twosson/kubeapt

# Usage data
set_telemetry_env 'DASH_DISABLE_TELEMETRY' 'ANY_VALUE'

# MSLab
# https://github.com/microsoft/MSLab

# Usage data
set_telemetry_env 'MSLAB_TELEMETRY_LEVEL' 'None'

# Nuke
# https://nuke.build/

# Usage data
set_telemetry_env 'NUKE_TELEMETRY_OPTOUT' '1'

# Packer
# https://www.packer.io/

# Update check
set_telemetry_env 'CHECKPOINT_DISABLE' '1'

# PnP PowerShell
# https://pnp.github.io/powershell/

# Usage data (env. var)
# https://pnp.github.io/powershell/articles/configuration.html#disable-or-enable-telemetry
set_telemetry_env 'PNPPOWERSHELL_DISABLETELEMETRY' 'true'

# PnP PowerShell
# https://pnp.github.io/powershell/

# Update check
# https://pnp.github.io/powershell/articles/updatenotifications.html
set_telemetry_env 'PNPPOWERSHELL_UPDATECHECK' 'false'

# Pulumi
# https://www.pulumi.com/

# Update check
set_telemetry_env 'PULUMI_SKIP_UPDATE_CHECK' 'true'

# Telepresence
# https://www.telepresence.io/

# Usage data
set_telemetry_env 'SCOUT_DISABLE' '1'

# Terraform
# https://www.terraform.io/

# Update check
# https://www.terraform.io/docs/commands/index.html#disable_checkpoint
set_telemetry_env 'CHECKPOINT_DISABLE' 'ANY_VALUE'

# Terraform Provider for Azure
# https://registry.terraform.io/providers/hashicorp/azurerm/latest

# Usage data
# https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#disable_terraform_partner_id
set_telemetry_env 'ARM_DISABLE_TERRAFORM_PARTNER_ID' 'true'

# Cloud Development Kit for Terraform
# https://github.com/hashicorp/terraform-cdk

# Usage data
set_telemetry_env 'CHECKPOINT_DISABLE' 'ANY_VALUE'

# Vagrant
# https://www.vagrantup.com/

# Vagrant update check
# https://www.vagrantup.com/docs/other/environmental-variables#vagrant_checkpoint_disable
set_telemetry_env 'VAGRANT_CHECKPOINT_DISABLE' 'ANY_VALUE'

# Vagrant
# https://www.vagrantup.com/

# Vagrant box update check
# https://www.vagrantup.com/docs/other/environmental-variables#vagrant_box_update_check_disable
set_telemetry_env 'VAGRANT_BOX_UPDATE_CHECK_DISABLE' 'ANY_VALUE'

# Weave Net
# https://www.weave.works/

# Update check
set_telemetry_env 'CHECKPOINT_DISABLE' '1'

# werf
# https://werf.io/

# Usage data
set_telemetry_env 'WERF_TELEMETRY' '0'

# WKSctl
# https://www.weave.works/oss/wksctl/

# Update check
set_telemetry_env 'CHECKPOINT_DISABLE' '1'

# AccessMap
# https://www.accessmap.io/

# Usage data
set_telemetry_env 'ANALYTICS' 'no'

# Oh My Zsh
# https://ohmyz.sh/

# Update check
set_telemetry_env 'DISABLE_AUTO_UPDATE' 'true'

# PowerShell Core
# https://github.com/powershell/powershell

# Usage data
# https://docs.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_telemetry
set_telemetry_env 'POWERSHELL_TELEMETRY_OPTOUT' '1'

# PowerShell Core
# https://github.com/powershell/powershell

# Update check
# https://docs.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_update_notification
set_telemetry_env 'POWERSHELL_UPDATECHECK' 'Off'

# Azure Application Insights (VSCode)
# https://marketplace.visualstudio.com/items?itemName=VisualStudioOnlineApplicationInsights.application-insights

# Usage data
set_telemetry_env 'AITOOLSVSCODE_DISABLETELEMETRY' 'ANY_VALUE'

# JavaScript debugger (VSCode)
# https://marketplace.visualstudio.com/items?itemName=ms-vscode.js-debug

# Usage data
# https://github.com/microsoft/vscode-js-debug/blob/12ec6df97f45b25b168e1eac8a17b802af73806f/src/ioc.ts#L168
set_telemetry_env 'DA_TEST_DISABLE_TELEMETRY' '1'

# Unset the function
unset -f set_telemetry_env