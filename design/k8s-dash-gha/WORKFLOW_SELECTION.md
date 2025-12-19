# Workflow Selection Guide

Quick reference to choose the right workflow for your needs.

## 📊 Comparison Matrix

| Feature | Simple | Bash | Multi-Env | Terraform | TF Optimized | Rollback | Security | Operations |
|---------|--------|------|-----------|-----------|--------------|----------|----------|------------|
| **Complexity** | ⭐ | ⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ | ⭐⭐ |
| **Speed** | 🚀🚀🚀 | 🚀🚀 | 🚀 | 🚀🚀 | 🚀🚀🚀 | 🚀🚀 | 🚀 | 🚀🚀 |
| **Production Ready** | ✅ | ✅✅ | ✅✅✅ | ✅✅ | ✅✅✅ | ✅✅✅ | ✅✅ | ✅✅ |
| **Manual Approval** | ❌ | Optional | ✅ | Optional | ✅ | ✅ | Optional | Optional |
| **Auto Backup** | ❌ | ❌ | ✅ | ❌ | ❌ | ✅ | ❌ | ✅ |
| **State Management** | N/A | N/A | Helm | Terraform | Terraform | Helm | N/A | N/A |
| **Multi-Environment** | ❌ | ✅ | ✅✅✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Caching** | ❌ | ❌ | ❌ | ❌ | ✅✅ | ❌ | ❌ | ❌ |
| **Rollback Capability** | ❌ | ❌ | ❌ | ✅ | ✅ | ✅✅✅ | ❌ | ❌ |
| **Security Scanning** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅✅✅ | ❌ |
| **Cost** | $ | $ | $$ | $$ | $ | $ | $ | $ |

**Legend:**
- ⭐ = Complexity level (more stars = more complex)
- 🚀 = Speed (more rockets = faster)
- ✅ = Production readiness (more checks = better for production)
- $ = Runner time cost (more $ = more expensive)

---

## 🎯 Decision Tree

```
What do you need to do?

┌─ Deploy Dashboard?
│  ├─ First time / Testing?
│  │  └─ USE: deploy-simple.yml
│  │
│  ├─ Development deployment?
│  │  └─ USE: deploy-bash.yml
│  │
│  ├─ Production deployment?
│  │  ├─ New to Terraform?
│  │  │  └─ USE: deploy-bash.yml or deploy-multi-env.yml
│  │  │
│  │  ├─ Using Terraform already?
│  │  │  └─ USE: deploy-terraform-optimized.yml
│  │  │
│  │  └─ Want maximum safety?
│  │     └─ USE: deploy-multi-env.yml
│  │
│  └─ Deploy to multiple environments?
│     └─ USE: deploy-multi-env.yml
│
├─ Fix broken deployment?
│  └─ USE: rollback.yml
│
├─ Security / Compliance?
│  ├─ Token expired?
│  │  └─ USE: security-token-rotation.yml
│  │
│  ├─ Security audit needed?
│  │  └─ USE: security-token-rotation.yml (scan_security: true)
│  │
│  └─ RBAC review?
│     └─ USE: security-token-rotation.yml (audit_rbac: true)
│
└─ Maintenance?
   ├─ Check health?
   │  └─ USE: operations.yml (health-check)
   │
   ├─ Backup before changes?
   │  └─ USE: operations.yml (backup)
   │
   ├─ Scale up/down?
   │  └─ USE: operations.yml (scale)
   │
   ├─ Restart pods?
   │  └─ USE: operations.yml (restart)
   │
   └─ Upgrade version?
      └─ USE: operations.yml (upgrade)
```

---

## 🔍 Detailed Scenarios

### Scenario 1: Quick Dev Test

**Goal:** Test configuration change quickly

**Workflow:** `deploy-simple.yml`

**Why:**
- Fastest deployment (2-3 minutes)
- No approvals needed
- Single click to deploy
- Perfect for iteration

**Steps:**
```
1. Actions → Quick Deploy Dashboard
2. Environment: dev
3. Method: helm
4. Deploy
```

---

### Scenario 2: First Production Deployment

**Goal:** Deploy dashboard to production for first time

**Workflow:** `deploy-bash.yml` or `deploy-multi-env.yml`

**Why:**
- Proven bash scripts
- Built-in validation
- Easy to understand
- No Terraform knowledge needed

**Recommendation:** Use `deploy-multi-env.yml` for extra safety

**Steps:**
```
1. Configure environments (dev, staging, prod)
2. Add approvers to prod environment
3. Push to main branch
4. Approve prod deployment when ready
```

---

### Scenario 3: Regular Production Deployments

**Goal:** Standard production deployment workflow

**Workflow:** `deploy-terraform-optimized.yml`

**Why:**
- Infrastructure as Code
- State management
- Fastest for regular deployments (caching)
- PR previews
- Easy rollback

**Steps:**
```
1. Create PR with changes
2. Review Terraform plan in PR
3. Merge PR
4. Workflow runs plan
5. Approve production apply
6. Automatic deployment
```

---

### Scenario 4: Multi-Team Safe Deployment

**Goal:** Deploy with multiple checkpoints and team reviews

**Workflow:** `deploy-multi-env.yml`

**Why:**
- Progressive deployment (dev → staging → prod)
- Multiple approval gates possible
- Automatic backups
- Smoke tests at each stage
- Maximum safety

**Steps:**
```
1. Push to main
2. Dev deploys automatically
3. Staging deploys automatically
4. Production waits for approval
5. Team reviews
6. Approve and deploy
```

---

### Scenario 5: Emergency Rollback

**Goal:** Service is down, need immediate rollback

**Workflow:** `rollback.yml`

**Why:**
- Fast recovery
- Multiple rollback methods
- Preserves history
- Creates backup before rollback

**Steps:**
```
1. Actions → Rollback Dashboard
2. Environment: prod
3. Rollback type: helm-history
4. Confirm: ROLLBACK
5. Execute
```

**Time to recovery:** 2-3 minutes

---

### Scenario 6: Daily Security Operations

**Goal:** Maintain security posture

**Workflow:** `security-token-rotation.yml`

**Why:**
- Automated daily token rotation
- RBAC audits
- Security scanning
- Compliance checks

**Configuration:**
```yaml
# Runs automatically daily at 2 AM
# Manual trigger for immediate rotation
```

---

### Scenario 7: Pre-Production Maintenance

**Goal:** Prepare for major changes

**Workflow:** `operations.yml`

**Actions:**
```
1. Backup current state
   - Operation: backup
   - Save run number

2. Health check before
   - Operation: health-check
   - Verify all healthy

3. Make changes (using deployment workflow)

4. Health check after
   - Operation: health-check

5. If issues → Restore
   - Operation: restore
   - Use backup run number
```

---

## 💡 Best Practices by Role

### Developers

**Daily Work:**
- Use: `deploy-simple.yml` for quick tests
- Use: `deploy-bash.yml` for dev deployments
- Always create PRs for production changes

**Recommended Flow:**
```
Feature branch → deploy-simple.yml (dev test)
  ↓
Create PR → deploy-bash.yml (validation)
  ↓
Merge → deploy-multi-env.yml (all environments)
```

---

### DevOps Engineers

**Infrastructure Changes:**
- Use: `deploy-terraform-optimized.yml`
- Always run plan before apply
- Review state changes carefully

**Operations:**
- Daily: `security-token-rotation.yml` (automated)
- Weekly: `operations.yml` (health-check)
- Before changes: `operations.yml` (backup)
- After hours: `operations.yml` (scale)

**Recommended Setup:**
```yaml
# .github/workflows/schedule.yml
on:
  schedule:
    - cron: '0 2 * * *'  # Daily security
    - cron: '0 8 * * 1'  # Weekly health check
```

---

### Platform Engineers

**Production Releases:**
- Use: `deploy-terraform-optimized.yml` (standard)
- Use: `deploy-multi-env.yml` (high-risk changes)
- Always: `rollback.yml` ready for emergencies

**Security:**
- Monthly: RBAC audit
- Quarterly: Security scan
- Always: Token rotation

**Monitoring:**
- Set up workflow failure alerts
- Track deployment frequency
- Monitor rollback rate

---

### SRE Team

**Incident Response:**
```
Issue detected
  ↓
operations.yml (health-check) → Diagnose
  ↓
rollback.yml → Recover
  ↓
operations.yml (health-check) → Verify
  ↓
Post-mortem → Update runbooks
```

**Capacity Planning:**
- Use: `operations.yml` (scale)
- Monitor resource usage
- Adjust replicas as needed

---

## 📈 Workflow Performance

### Speed Comparison

| Workflow | First Run | Cached Run | Use Case |
|----------|-----------|------------|----------|
| Simple | 2-3 min | 2-3 min | Dev testing |
| Bash | 3-4 min | 3-4 min | Standard deploys |
| Multi-Env | 8-12 min | 8-12 min | Staged rollout |
| Terraform | 4-5 min | 4-5 min | IaC standard |
| TF Optimized | 4-5 min | 2-3 min | IaC with cache |
| Rollback | 2-3 min | 2-3 min | Emergency |
| Security | 5-7 min | 5-7 min | Audits |
| Operations | 1-5 min | 1-5 min | Maintenance |

**Winner:** `deploy-terraform-optimized.yml` for regular deployments (cached)

---

## 💰 Cost Analysis

### GitHub Actions Minutes (per deployment)

| Workflow | Minutes | Cost/Deploy* | Monthly Cost** |
|----------|---------|--------------|----------------|
| Simple | 3 | $0.024 | $0.72 |
| Bash | 4 | $0.032 | $0.96 |
| Multi-Env | 12 | $0.096 | $2.88 |
| Terraform | 5 | $0.040 | $1.20 |
| TF Optimized | 3 | $0.024 | $0.72 |
| Rollback | 3 | $0.024 | $0.07 (rare) |
| Security | 6 | $0.048 | $1.44 (daily) |
| Operations | 2 | $0.016 | $0.32 (weekly) |

*Based on $0.008/minute for Ubuntu runner
**Assumes: 1 deploy/day (30/month), except where noted

**Total Monthly Cost:** ~$8-10 for typical usage

**Winner:** `deploy-simple.yml` and `deploy-terraform-optimized.yml` (cached)

---

## 🎓 Learning Path

### Beginner (Week 1)

1. **Start:** `deploy-simple.yml`
   - Learn basic deployment
   - Understand workflow structure
   - Practice in dev environment

2. **Progress:** `operations.yml`
   - Learn maintenance tasks
   - Understand health checks
   - Practice troubleshooting

### Intermediate (Week 2-3)

3. **Learn:** `deploy-bash.yml`
   - Understand script-based deployment
   - Learn validation steps
   - Practice multi-environment

4. **Explore:** `deploy-multi-env.yml`
   - Understand staged rollout
   - Learn approval gates
   - Practice production safety

### Advanced (Week 4+)

5. **Master:** `deploy-terraform-optimized.yml`
   - Learn infrastructure as code
   - Understand state management
   - Practice advanced deployments

6. **Specialize:** `security-token-rotation.yml` + `rollback.yml`
   - Learn security automation
   - Master incident response
   - Practice emergency procedures

---

## 📝 Quick Reference Card

**Print this and keep at your desk!**

```
┌─────────────────────────────────────────────┐
│  KUBERNETES DASHBOARD WORKFLOW QUICK REF    │
├─────────────────────────────────────────────┤
│ DEPLOY TO DEV                               │
│   → deploy-simple.yml                       │
│   → 2 min                                   │
│                                             │
│ DEPLOY TO PROD                              │
│   → deploy-terraform-optimized.yml          │
│   → 3 min (cached)                          │
│                                             │
│ EMERGENCY ROLLBACK                          │
│   → rollback.yml                            │
│   → Type: helm-history                      │
│   → Confirm: ROLLBACK                       │
│                                             │
│ ROTATE TOKENS                               │
│   → security-token-rotation.yml (auto)      │
│   → Runs daily at 2 AM UTC                  │
│                                             │
│ HEALTH CHECK                                │
│   → operations.yml                          │
│   → Operation: health-check                 │
│                                             │
│ BACKUP BEFORE CHANGES                       │
│   → operations.yml                          │
│   → Operation: backup                       │
│   → Save run number!                        │
└─────────────────────────────────────────────┘
```

---

## ✅ Checklist: Am I Using the Right Workflow?

### For Development
- [ ] Using deploy-simple.yml for quick tests?
- [ ] Using deploy-bash.yml for dev deploys?
- [ ] Creating PRs for validation?

### For Production
- [ ] Using deploy-terraform-optimized.yml?
- [ ] OR using deploy-multi-env.yml for safety?
- [ ] Environment approvals configured?
- [ ] Backup before major changes?

### For Security
- [ ] Token rotation automated?
- [ ] Monthly RBAC audits scheduled?
- [ ] Security scans before prod?

### For Operations
- [ ] Weekly health checks?
- [ ] Backup strategy defined?
- [ ] Rollback process tested?

---

**Need Help?** See [Workflows README](.github/workflows/README.md) for detailed documentation.

**Quick Setup?** See [Quick Setup Guide](.github/workflows/QUICK_SETUP.md) for 15-minute setup.
