locals {
  name_prefix = "${var.project}-${var.environment}"

  tags = merge(
    {
      Project     = var.project
      Environment = var.environment
      ManagedBy   = "terraform"
      Service     = "container-provisioner"
    },
    var.common_tags
  )

  oidc_issuer_hostpath = replace(var.eks_oidc_issuer_url, "https://", "")
}

check "irsa_inputs" {
  assert {
    condition = (
      !var.create_irsa_role ||
      (var.eks_oidc_provider_arn != "" && var.eks_oidc_issuer_url != "")
    )
    error_message = "create_irsa_role=true 이면 eks_oidc_provider_arn, eks_oidc_issuer_url 값을 모두 설정해야 합니다."
  }
}

resource "aws_dynamodb_table" "stacks" {
  name         = var.dynamodb_table_name
  billing_mode = var.dynamodb_billing_mode

  read_capacity  = var.dynamodb_billing_mode == "PROVISIONED" ? var.dynamodb_read_capacity : null
  write_capacity = var.dynamodb_billing_mode == "PROVISIONED" ? var.dynamodb_write_capacity : null

  hash_key  = "pk"
  range_key = "sk"

  attribute {
    name = "pk"
    type = "S"
  }

  attribute {
    name = "sk"
    type = "S"
  }

  attribute {
    name = "gsi1pk"
    type = "S"
  }

  attribute {
    name = "gsi1sk"
    type = "S"
  }

  global_secondary_index {
    name            = "gsi1"
    hash_key        = "gsi1pk"
    range_key       = "gsi1sk"
    projection_type = "ALL"

    read_capacity  = var.dynamodb_billing_mode == "PROVISIONED" ? var.dynamodb_read_capacity : null
    write_capacity = var.dynamodb_billing_mode == "PROVISIONED" ? var.dynamodb_write_capacity : null
  }

  point_in_time_recovery {
    enabled = var.enable_point_in_time_recovery
  }

  server_side_encryption {
    enabled = true
  }
}

data "aws_iam_policy_document" "dynamodb_access" {
  statement {
    sid = "DynamoDBAccessForContainerProvisioner"

    actions = [
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:UpdateItem",
      "dynamodb:DeleteItem",
      "dynamodb:Query",
      "dynamodb:TransactWriteItems"
    ]

    resources = [
      aws_dynamodb_table.stacks.arn,
      "${aws_dynamodb_table.stacks.arn}/index/*"
    ]
  }
}

resource "aws_iam_policy" "app_dynamodb" {
  name        = "${local.name_prefix}-container-provisioner-ddb"
  description = "DynamoDB access policy for container-provisioner"
  policy      = data.aws_iam_policy_document.dynamodb_access.json
}

data "aws_iam_policy_document" "irsa_assume" {
  count = var.create_irsa_role ? 1 : 0

  statement {
    sid     = "IRSAAssumeRole"
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [var.eks_oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_issuer_hostpath}:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_issuer_hostpath}:sub"
      values   = ["system:serviceaccount:${var.k8s_service_account_namespace}:${var.k8s_service_account_name}"]
    }
  }
}

resource "aws_iam_role" "irsa" {
  count              = var.create_irsa_role ? 1 : 0
  name               = var.irsa_role_name
  assume_role_policy = data.aws_iam_policy_document.irsa_assume[0].json
}

resource "aws_iam_role_policy_attachment" "irsa_dynamodb" {
  count      = var.create_irsa_role ? 1 : 0
  role       = aws_iam_role.irsa[0].name
  policy_arn = aws_iam_policy.app_dynamodb.arn
}
