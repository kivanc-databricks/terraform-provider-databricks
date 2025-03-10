---
page_title: "Provisioning AWS Databricks E2"
---

# Provisioning AWS Databricks E2

You can provision multiple Databricks workspaces with Terraform.

![Simplest multiworkspace](https://github.com/databrickslabs/terraform-provider-databricks/raw/master/docs/simplest-multiworkspace.png)

## Provider initialization for E2 workspaces

This guide assumes you have `databricks_account_username` and `databricks_account_password` for [https://accounts.cloud.databricks.com](https://accounts.cloud.databricks.com) and can find `databricks_account_id` in the bottom left corner of the page, once you're logged in. This guide is provided as is and assumes you'll use it as the basis for your setup.

```hcl
variable "databricks_account_username" {}
variable "databricks_account_password" {}
variable "databricks_account_id" {}

variable "tags" {
  default = {}
}

variable "cidr_block" {
  default = "10.4.0.0/16"
}

variable "region" {
  default = "eu-west-1"
}

resource "random_string" "naming" {
  special = false
  upper   = false
  length  = 6
}

locals {
  prefix = "demo${random_string.naming.result}"
}
```

Before [managing workspace](workspace-management.md), you have to create:
  - [VPC](#vpc)
  - [Root bucket](#root-bucket)
  - [Cross-account role](#cross-account-iam-role)
  - [Databricks E2 workspace](#databricks-e2-workspace)
  - [Host and Token outputs](#provider-configuration)

> Initialize provider with `alias = "mws"` and use `provider = databricks.mws` for all `databricks_mws_*` resources. We require all `databricks_mws_*` resources to be created within its own dedicated terraform module of your environment. Usually this module creates VPC and IAM roles as well.

```hcl
terraform {
  required_providers {
    databricks = {
      source  = "databrickslabs/databricks"
      version = "0.3.11"
    }
    aws = {
      source = "hashicorp/aws"
      version = "3.49.0"
    }
  }
}

provider "aws" {
  region = var.region
}

// initialize provider in "MWS" mode to provision new workspace
provider "databricks" {
  alias    = "mws"
  host     = "https://accounts.cloud.databricks.com"
  username = var.databricks_account_username
  password = var.databricks_account_password
}
```

## Cross-account IAM Role

Cross-account IAM role is registered with [databricks_mws_credentials](../resources/mws_credentials.md) resource.

```hcl
data "databricks_aws_assume_role_policy" "this" {
  external_id = var.databricks_account_id
}

resource "aws_iam_role" "cross_account_role" {
  name               = "${local.prefix}-crossaccount"
  assume_role_policy = data.databricks_aws_assume_role_policy.this.json
  tags               = var.tags
}

data "databricks_aws_crossaccount_policy" "this" {
}

resource "aws_iam_role_policy" "this" {
  name   = "${local.prefix}-policy"
  role   = aws_iam_role.cross_account_role.id
  policy = data.databricks_aws_crossaccount_policy.this.json
}

resource "databricks_mws_credentials" "this" {
  provider         = databricks.mws
  account_id       = var.databricks_account_id
  role_arn         = aws_iam_role.cross_account_role.arn
  credentials_name = "${local.prefix}-creds"
  depends_on       = [aws_iam_role_policy.this]
}
```

## VPC

The very first step is VPC creation with necessary firewall rules. Please consult [main documetation page](https://docs.databricks.com/administration-guide/cloud-configurations/aws/customer-managed-vpc.html) for **the most complete and up-to-date details on networking**. AWS VPS is registered as [databricks_mws_networks](../resources/mws_networks.md) resource. For STS, S3 and Kinesis, you can create VPC gateway or interface endpoints such that the relevant in-region traffic from clusters could transit over the secure AWS backbone rather than the public network, for more direct connections and reduced cost compared to AWS global endpoints. For more information, see [Regional endpoints](https://docs.databricks.com/administration-guide/cloud-configurations/aws/customer-managed-vpc.html#regional-endpoints-1).

```hcl
data "aws_availability_zones" "available" {}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "3.2.0"

  name = local.prefix
  cidr = var.cidr_block
  azs  = data.aws_availability_zones.available.names
  tags = var.tags

  enable_dns_hostnames = true
  enable_nat_gateway   = true
  single_nat_gateway   = true
  create_igw           = true

  public_subnets  = [cidrsubnet(var.cidr_block, 3, 0)]
  private_subnets = [cidrsubnet(var.cidr_block, 3, 1),
                     cidrsubnet(var.cidr_block, 3, 2)]

  manage_default_security_group = true
  default_security_group_name = "${local.prefix}-sg"

  default_security_group_egress = [{
    cidr_blocks = "0.0.0.0/0"
  }]

  default_security_group_ingress = [{
    description = "Allow all internal TCP and UDP"
    self        = true
  }]
}

module "vpc_endpoints" {
  source = "terraform-aws-modules/vpc/aws//modules/vpc-endpoints"
  version = "3.2.0"

  vpc_id             = module.vpc.vpc_id
  security_group_ids = [module.vpc.default_security_group_id]

  endpoints = {
    s3 = {
      service         = "s3"
      service_type    = "Gateway"
      route_table_ids = flatten([
        module.vpc.private_route_table_ids,
        module.vpc.public_route_table_ids])
      tags            = {
        Name = "${local.prefix}-s3-vpc-endpoint"
      }
    },
    sts = {
      service             = "sts"
      private_dns_enabled = true
      subnet_ids          = module.vpc.private_subnets
      tags                = {
        Name = "${local.prefix}-sts-vpc-endpoint"
      }
    },
    kinesis-streams = {
      service             = "kinesis-streams"
      private_dns_enabled = true
      subnet_ids          = module.vpc.private_subnets
      tags                = {
        Name = "${local.prefix}-kinesis-vpc-endpoint"
      }
    },
  }

  tags = var.tags
}

resource "databricks_mws_networks" "this" {
  provider           = databricks.mws
  account_id         = var.databricks_account_id
  network_name       = "${local.prefix}-network"
  security_group_ids = [module.vpc.default_security_group_id]
  subnet_ids         = module.vpc.private_subnets
  vpc_id             = module.vpc.vpc_id
}
```

## Root bucket

Once [VPC](#vpc) is ready, create AWS S3 bucket for DBFS workspace storage, which is commonly referred to as **root bucket**. This provider has [databricks_aws_bucket_policy](../data-sources/aws_bucket_policy.md) with the necessary IAM policy template. AWS S3 bucket has to be registered through [databricks_mws_storage_configurations](../resources/mws_storage_configurations.md).

```hcl
resource "aws_s3_bucket" "root_storage_bucket" {
  bucket = "${local.prefix}-rootbucket"
  acl    = "private"
  versioning {
    enabled = false
  }
  force_destroy = true
  tags = merge(var.tags, {
    Name = "${local.prefix}-rootbucket"
  })
}

resource "aws_s3_bucket_public_access_block" "root_storage_bucket" {
  bucket             = aws_s3_bucket.root_storage_bucket.id
  ignore_public_acls = true
  depends_on         = [aws_s3_bucket.root_storage_bucket]
}

data "databricks_aws_bucket_policy" "this" {
  bucket = aws_s3_bucket.root_storage_bucket.bucket
}

resource "aws_s3_bucket_policy" "root_bucket_policy" {
  bucket     = aws_s3_bucket.root_storage_bucket.id
  policy     = data.databricks_aws_bucket_policy.this.json
  depends_on = [aws_s3_bucket_public_access_block.root_storage_bucket]
}

resource "databricks_mws_storage_configurations" "this" {
  provider                   = databricks.mws
  account_id                 = var.databricks_account_id
  bucket_name                = aws_s3_bucket.root_storage_bucket.bucket
  storage_configuration_name = "${local.prefix}-storage"
}
```

## Databricks E2 Workspace

Once  [VPC](#vpc), [cross-account role](#cross-account-iam-role), and [root bucket](#root-bucket) are setup, you can create Databricks AWS E2 workspace through [databricks_mws_workspaces](../resources/mws_workspaces.md) resource.

Code that creates workspaces and code that [manages workspaces](workspace-management.md) must be in separate terraform modules to avoid common confusion between `provider = databricks.mws` and `provider = databricks.created_workspace`. This is why we specify `databricks_host` and `databricks_token` outputs, that have to be used in the latter modules.

-> **Note** If you experience technical difficulties with rolling out resources in this example, please make sure that [environment variables](../index.md#environment-variables) don't [conflict with other](../index.md#empty-provider-block) provider block attributes. When in doubt, please run `TF_LOG=DEBUG terraform apply` to enable [debug mode](https://www.terraform.io/docs/internals/debugging.html) through the [`TF_LOG`](https://www.terraform.io/docs/cli/config/environment-variables.html#tf_log) environment variable. Look specifically for `Explicit and implicit attributes` lines, that should indicate authentication attributes used. The other common reason for technical difficulties might be related to missing `alias` attribute in `provider "databricks" {}` blocks or `provider` attribute in `resource "databricks_..." {}` blocks. Please make sure to read [`alias`: Multiple Provider Configurations](https://www.terraform.io/docs/language/providers/configuration.html#alias-multiple-provider-configurations) documentation article.

```hcl
resource "databricks_mws_workspaces" "this" {
  provider        = databricks.mws
  account_id      = var.databricks_account_id
  aws_region      = var.region
  workspace_name  = local.prefix
  deployment_name = local.prefix

  credentials_id           = databricks_mws_credentials.this.credentials_id
  storage_configuration_id = databricks_mws_storage_configurations.this.storage_configuration_id
  network_id               = databricks_mws_networks.this.network_id
}

output "databricks_host" {
  value = databricks_mws_workspaces.this.workspace_url
}

// initialize provider in normal mode
provider "databricks" {
  // in normal scenario you won't have to give providers aliases
  alias = "created_workspace"
  host = databricks_mws_workspaces.this.workspace_url
}

// create PAT token to provision entities within workspace
resource "databricks_token" "pat" {
  provider = databricks.created_workspace
  comment  = "Terraform Provisioning"
  lifetime_seconds = 86400
}

// export token for integration tests to run on
output "databricks_token" {
  value     = databricks_token.pat.token_value
  sensitive = true
}
```

### Data resources and Authentication is not configured errors

*In Terraform 0.13 and later*, data resources have the same dependency resolution behavior [as defined for managed resources](https://www.terraform.io/docs/language/resources/behavior.html#resource-dependencies). Most data resources make an API call to a workspace. If a workspace doesn't exist yet, `authentication is not configured for provider` error is raised. To work around this issue and guarantee a proper lazy authentication with data resources, you should add `depends_on = [databricks_mws_workspaces.this]` to the body. This issue doesn't occur if workspace is created *in one module* and resources [within the workspace](workspace-management.md) are created *in another*. We do not recommend using Terraform 0.12 and earlier, if your usage involves data resources.

```hcl
data "databricks_current_user" "me" {
  depends_on = [databricks_mws_workspaces.this]
}
```

## Provider configuration

In [the next step](workspace-management.md), please use the following configuration for the provider:

```hcl
provider "databricks" {
  host = module.ai.databricks_host
  token = module.ai.databricks_token
}
```

### Credentials validation checks errors

Due to a bug in the Terraform AWS provider (spotted in v3.28) the Databricks AWS crossaccount policy creation and attachment to the IAM role takes longer than the AWS request confirmation to Terraform. As Terraform continues creating the Workspace, validation checks for the credentials are failing, as the policy doesn't get applied quick enough. Showing the error:

```
Error: MALFORMED_REQUEST: Failed credentials validation checks: Spot Cancellation, Create Placement Group, Delete Tags, Describe Availability Zones, Describe instances, Describe Instance Status, Describe Placement Group, Describe Route Tables, Describe Security Groups, Describe Spot Instances, Describe Spot Price History, Describe Subnets, Describe Volumes, Describe Vpcs, Request Spot Instances
(400 on /api/2.0/accounts/{UUID}/workspaces)
```

As a workaround give the `aws_iam_role` more time to be created with a `time_sleep` resource, which you need to add as a dependency to the `databricks_mws_workspaces` resource.

```hcl
resource "time_sleep" "wait" {
  depends_on = [
    aws_iam_role.cross_account_role]
  create_duration = "10s"
}
```

#### IAM policy error

If you notice below error:

```
Error: MALFORMED_REQUEST: Failed credentials validation checks: Spot Cancellation, Create Placement Group, Delete Tags, Describe Availability Zones, Describe instances, Describe Instance Status, Describe Placement Group, Describe Route Tables, Describe Security Groups, Describe Spot Instances, Describe Spot Price History, Describe Subnets, Describe Volumes, Describe Vpcs, Request Spot Instances
```

- Try creating workspace from UI:

![create_workspace_error](https://github.com/databrickslabs/terraform-provider-databricks/raw/master/docs/images/create_workspace_error.png)


- Verify if the role and policy exists (assume role should allow external id)

![iam_role_trust_error](https://github.com/databrickslabs/terraform-provider-databricks/raw/master/docs/images/iam_role_trust_error.png)

