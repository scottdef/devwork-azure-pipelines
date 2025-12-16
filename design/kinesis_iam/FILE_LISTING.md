# Bedrock to Dynatrace Logging - Complete Package

## 📦 Package Overview

This is a complete, production-ready solution for streaming Amazon Bedrock model invocation logs to Dynatrace for monitoring and analysis.

**Total Files:** 18 files across multiple categories

---

## 📁 File Structure

```
bedrock-dynatrace-logging/
├── 📄 Documentation (6 files)
│   ├── GETTING_STARTED.md              # Quick start guide - START HERE
│   ├── README.md                       # Complete documentation
│   ├── QUICK_REFERENCE.md              # Command reference
│   ├── IAM_PERMISSIONS_GUIDE.md        # Detailed IAM permissions guide
│   ├── FILE_LISTING.md                 # This file
│   └── iam-policies/README.md          # IAM policies documentation
│
├── 🔧 Terraform Configuration (6 files)
│   ├── main.tf                         # Main infrastructure resources
│   ├── variables.tf                    # Input variables
│   ├── outputs.tf                      # Output values
│   ├── iam.tf                          # IAM roles and policies
│   ├── lambda.tf                       # Lambda function config
│   └── dashboard.tf                    # CloudWatch dashboard
│
├── 🐍 Lambda Code (1 file)
│   └── lambda/transformer.py           # Log transformation code
│
├── 🔐 IAM Policies (3 files)
│   ├── iam-policies/deployment-policy.json    # Recommended for production
│   ├── iam-policies/minimal-policy.json       # Minimal permissions
│   └── iam-policies/admin-policy.json         # Admin (dev/test only)
│
├── 🚀 Helper Scripts (3 files)
│   ├── setup.sh                        # Automated deployment
│   ├── apply-iam-policy.sh             # IAM policy setup helper
│   └── test_logging.py                 # Test script
│
└── ⚙️ Configuration (2 files)
    ├── terraform.tfvars.example        # Example configuration
    └── .gitignore                      # Git ignore rules
```

---

## 🎯 Where to Start

### For First-Time Users
1. **Read:** `GETTING_STARTED.md` ← Start here!
2. **Configure IAM:** `./apply-iam-policy.sh` or manually with `iam-policies/`
3. **Configure Terraform:** Copy `terraform.tfvars.example` → `terraform.tfvars`
4. **Deploy:** Run `./setup.sh`
5. **Test:** Run `./test_logging.py`

### For IAM/Security Review
1. **Read:** `IAM_PERMISSIONS_GUIDE.md` - Complete IAM documentation
2. **Review:** `iam-policies/README.md` - Policy comparison
3. **Choose:** `deployment-policy.json` (recommended) or `minimal-policy.json`

### For Operations/DevOps
1. **Reference:** `QUICK_REFERENCE.md` - Commands and queries
2. **Deploy:** `setup.sh` - Automated deployment
3. **Test:** `test_logging.py` - Pipeline verification
4. **Monitor:** Use Dynatrace queries from the quick reference

---

## 📄 Documentation Files

### GETTING_STARTED.md
**Purpose:** Quick start guide for new users  
**Length:** ~350 lines  
**Contains:**
- Prerequisites checklist
- 5-minute quick start
- Configuration examples
- Testing instructions
- Cost estimates
- Troubleshooting tips

**When to use:** You're deploying this for the first time

---

### README.md
**Purpose:** Complete technical documentation  
**Length:** ~650 lines  
**Contains:**
- Architecture overview
- Detailed setup instructions
- Configuration options
- Dynatrace log queries
- Monitoring and alerts
- Cost analysis
- Troubleshooting guide
- Advanced configuration

**When to use:** You need detailed technical information

---

### QUICK_REFERENCE.md
**Purpose:** Command and query reference  
**Length:** ~350 lines  
**Contains:**
- Essential commands
- AWS CLI commands
- Dynatrace log queries
- Monitoring commands
- Troubleshooting steps
- Cost optimization tips

**When to use:** You need to look up a specific command or query

---

### IAM_PERMISSIONS_GUIDE.md
**Purpose:** Comprehensive IAM permissions documentation  
**Length:** ~850 lines  
**Contains:**
- Detailed permission requirements by service
- Service role configurations
- Minimal and admin policies
- Troubleshooting permission errors
- Security best practices
- Permission verification steps

**When to use:** You need to understand or troubleshoot IAM permissions

---

### iam-policies/README.md
**Purpose:** IAM policy directory documentation  
**Length:** ~250 lines  
**Contains:**
- Policy comparison
- Application instructions
- Quick start commands
- Customization examples
- Security best practices

**When to use:** You're setting up IAM policies

---

### FILE_LISTING.md
**Purpose:** Package overview and navigation  
**This file!**

---

## 🔧 Terraform Files

### main.tf
**Lines:** ~200  
**Creates:**
- CloudWatch Log Groups (Bedrock, Firehose, Lambda)
- S3 buckets (failed logs, large data)
- Kinesis Firehose delivery stream
- CloudWatch subscription filter
- Bedrock logging configuration

**Key resources:**
- `aws_cloudwatch_log_group.bedrock_logs`
- `aws_kinesis_firehose_delivery_stream.bedrock_to_dynatrace`
- `aws_s3_bucket.failed_logs`

---

### variables.tf
**Lines:** ~120  
**Defines:** All configurable parameters  
**Key variables:**
- AWS account/region
- Dynatrace URL and token
- Buffer settings
- Feature flags
- Logging options

---

### outputs.tf
**Lines:** ~75  
**Outputs:**
- Log group names and ARNs
- Firehose stream details
- S3 bucket names
- IAM role ARNs
- Setup status

---

### iam.tf
**Lines:** ~250  
**Creates:**
- Bedrock logging role
- CloudWatch to Firehose role
- Firehose delivery role
- Lambda execution role
- Associated policies

---

### lambda.tf
**Lines:** ~60  
**Creates:**
- Lambda function resource
- CloudWatch log group for Lambda
- Lambda permission for Firehose

---

### dashboard.tf
**Lines:** ~130  
**Creates:** Optional CloudWatch dashboard  
**Widgets:**
- Firehose metrics
- Lambda performance
- S3 storage
- Log queries

---

## 🐍 Lambda Code

### lambda/transformer.py
**Lines:** ~280  
**Purpose:** Transform Bedrock logs for Dynatrace  
**Features:**
- Parses CloudWatch Logs format
- Extracts Bedrock-specific fields
- Adds 15+ custom attributes
- Handles errors gracefully
- Supports compression

**Key functions:**
- `lambda_handler()` - Main entry point
- `transform_bedrock_log()` - Transform logic
- `process_cloudwatch_logs()` - CloudWatch parsing

---

## 🔐 IAM Policy Files

### deployment-policy.json (Recommended)
**Lines:** ~140  
**Use for:** Production deployments  
**Permissions:** Comprehensive, well-scoped  
**Services:** Bedrock, CloudWatch, Firehose, S3, Lambda, IAM, CloudWatch

---

### minimal-policy.json
**Lines:** ~70  
**Use for:** Strict security requirements  
**Permissions:** Absolute minimum  
**Note:** May require additional permissions in some environments

---

### admin-policy.json
**Lines:** ~15  
**Use for:** Development/testing only  
**Permissions:** Full admin on required services  
**⚠️ Warning:** Do not use in production

---

## 🚀 Helper Scripts

### setup.sh
**Lines:** ~200  
**Purpose:** Automated deployment  
**Features:**
- Prerequisite checking
- AWS credential validation
- Dynatrace connectivity test
- Terraform initialization
- Automated deployment
- Success verification

**Usage:**
```bash
chmod +x setup.sh
./setup.sh
```

---

### apply-iam-policy.sh
**Lines:** ~250  
**Purpose:** IAM policy setup helper  
**Features:**
- Interactive policy selection
- Policy creation
- Policy attachment
- Permission verification
- User-friendly interface

**Usage:**
```bash
chmod +x apply-iam-policy.sh
./apply-iam-policy.sh
```

---

### test_logging.py
**Lines:** ~250  
**Purpose:** Test the logging pipeline  
**Features:**
- Bedrock model invocation
- CloudWatch log verification
- Firehose metrics checking
- Dynatrace verification instructions

**Usage:**
```bash
chmod +x test_logging.py
./test_logging.py
```

---

## ⚙️ Configuration Files

### terraform.tfvars.example
**Lines:** ~40  
**Purpose:** Example configuration  
**Contains:**
- Required variables with defaults
- Feature flags
- Comments explaining each option

**Usage:**
```bash
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your values
```

---

### .gitignore
**Lines:** ~30  
**Purpose:** Git ignore patterns  
**Ignores:**
- Terraform state files
- Terraform variable files (except .example)
- Lambda deployment packages
- IDE files

---

## 🎯 Quick Navigation

| I want to... | Go to... |
|--------------|----------|
| Deploy for the first time | `GETTING_STARTED.md` |
| Understand the architecture | `README.md` → Architecture section |
| Set up IAM permissions | `apply-iam-policy.sh` or `iam-policies/` |
| Look up a command | `QUICK_REFERENCE.md` |
| Troubleshoot IAM issues | `IAM_PERMISSIONS_GUIDE.md` |
| Configure the deployment | `terraform.tfvars.example` |
| Understand Terraform resources | `main.tf`, `iam.tf`, `lambda.tf` |
| Test the pipeline | `test_logging.py` |
| Deploy automatically | `setup.sh` |
| View Dynatrace queries | `QUICK_REFERENCE.md` or `README.md` |

---

## 📊 Statistics

- **Total lines of code:** ~2,800
- **Total lines of documentation:** ~2,500
- **Terraform resources created:** 20+
- **IAM policies included:** 3
- **Helper scripts:** 3
- **Estimated deployment time:** 5-10 minutes
- **Estimated cost:** $5-10/month (10k invocations)

---

## 🔄 Deployment Workflow

```
1. Review Prerequisites (GETTING_STARTED.md)
   ↓
2. Set up IAM Permissions (apply-iam-policy.sh)
   ↓
3. Configure Variables (terraform.tfvars)
   ↓
4. Deploy Infrastructure (setup.sh)
   ↓
5. Test Pipeline (test_logging.py)
   ↓
6. Monitor in Dynatrace (QUICK_REFERENCE.md queries)
```

---

## 🆘 Troubleshooting Guide

| Issue | Check |
|-------|-------|
| Permission errors | `IAM_PERMISSIONS_GUIDE.md` |
| Deployment fails | `README.md` → Troubleshooting |
| Logs not appearing | `QUICK_REFERENCE.md` → Monitoring |
| Cost concerns | `README.md` → Cost Considerations |
| Configuration questions | `terraform.tfvars.example` comments |

---

## ✅ Pre-Deployment Checklist

Use this before running `setup.sh`:

- [ ] Read `GETTING_STARTED.md`
- [ ] AWS CLI installed and configured
- [ ] Terraform installed (v1.0+)
- [ ] IAM permissions applied (use `apply-iam-policy.sh`)
- [ ] Dynatrace API token created (with `logs.ingest` scope)
- [ ] `terraform.tfvars` created and configured
- [ ] Bedrock access enabled in your AWS account
- [ ] All executable permissions set (`chmod +x *.sh *.py`)

---

## 🎓 Learning Path

### Beginner
1. `GETTING_STARTED.md` - Understand what this does
2. `apply-iam-policy.sh` - Set up permissions
3. `setup.sh` - Deploy
4. `test_logging.py` - Verify

### Intermediate
1. `README.md` - Deep dive into architecture
2. `QUICK_REFERENCE.md` - Learn useful commands
3. `main.tf`, `iam.tf` - Understand infrastructure
4. Customize for your needs

### Advanced
1. `IAM_PERMISSIONS_GUIDE.md` - Master permissions
2. `lambda/transformer.py` - Customize transformation
3. `dashboard.tf` - Add custom metrics
4. Implement multi-region deployment

---

## 📞 Support Resources

- **Quick questions:** `QUICK_REFERENCE.md`
- **Detailed issues:** `README.md` → Troubleshooting
- **IAM problems:** `IAM_PERMISSIONS_GUIDE.md`
- **AWS Support:** For Bedrock/Firehose issues
- **Dynatrace Support:** For log ingestion issues

---

## 🚀 Next Steps After Deployment

1. ✅ Verify logs in Dynatrace: `log.source="aws.bedrock"`
2. 📊 Set up alerts for errors and high usage
3. 💰 Monitor costs in AWS Cost Explorer
4. 📈 Create custom dashboards in Dynatrace
5. 🔍 Explore log queries in `QUICK_REFERENCE.md`
6. 🔐 Review and tighten IAM permissions if needed
7. 📝 Document any custom changes for your team

---

**Version:** 1.0  
**Last Updated:** December 2024  
**Terraform Version:** >= 1.0  
**AWS Provider:** ~> 5.0  

For the latest documentation, see the individual files listed above.
