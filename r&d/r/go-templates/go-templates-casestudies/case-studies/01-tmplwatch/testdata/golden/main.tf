# Managed by tmplwatch. Do not edit by hand.
terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = "us-east-1"
}

locals {
  env           = "beta"
  allowed_cidrs = ["10.0.0.0/8", "192.168.0.0/16"]
}

data "aws_ami" "base" {
  most_recent = true
  owners      = ["amazon"]
  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }
}

resource "aws_instance" "web" {
  count         = 2
  ami           = data.aws_ami.base.id
  instance_type = "t3.small"

  tags = merge({ cost-center = "$${not.interpolated}", role = "web" }, {
    Name = "${local.env}-web-${count.index}"
  })
}

resource "aws_instance" "worker" {
  count         = 1
  ami           = data.aws_ami.base.id
  instance_type = "t3.large"

  tags = merge({ role = "worker" }, {
    Name = "${local.env}-worker-${count.index}"
  })
}

output "instance_ids" {
  value = {
    web    = aws_instance.web[*].id
    worker = aws_instance.worker[*].id
  }
}
