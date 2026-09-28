// Run with: node --test '.github/validator/tests/*.test.js'
import assert from 'node:assert/strict'
import { test } from 'node:test'

import budgetTarget from '../budget-target.js'
import costCenter from '../cost-center.js'
import date from '../date.js'
import deploymentName from '../deployment-name.js'
import logins from '../logins.js'
import noSecrets from '../no-secrets.js'
import positiveInt from '../positive-int.js'
import repositories from '../repositories.js'
import teams from '../teams.js'
import usdAmount from '../usd-amount.js'

// No ISSUEOPS_VALIDATOR_TOKEN in tests: API existence checks are skipped.
delete process.env.ISSUEOPS_VALIDATOR_TOKEN

test('usd-amount', async () => {
  assert.equal(await usdAmount('2500'), 'success')
  assert.equal(await usdAmount('$2,500'), 'success')
  assert.notEqual(await usdAmount('2500.50'), 'success')
  assert.notEqual(await usdAmount('0'), 'success')
  assert.notEqual(await usdAmount('lots'), 'success')
})

test('dates', async () => {
  assert.equal(await date(''), 'success')
  assert.equal(await date('_No response_'), 'success')
  assert.equal(await date('2026-12-31'), 'success')
  assert.notEqual(await date('2026-02-30'), 'success')
  assert.notEqual(await date('12/31/2026'), 'success')
})

test('cost centers and capacity', async () => {
  assert.equal(await costCenter('CC-4410-PAYMENTS'), 'success')
  assert.notEqual(await costCenter('payments'), 'success')
  assert.equal(await positiveInt('10'), 'success')
  assert.notEqual(await positiveInt('-1'), 'success')
})

test('names', async () => {
  assert.equal(await logins('dana-dev, @gia-pay'), 'success')
  assert.notEqual(await logins('dana dev'), 'success')
  assert.equal(await budgetTarget('@sam-contract'), 'success')
  assert.equal(await budgetTarget('CoolEngOrg/payments-api'), 'success')
  assert.equal(await deploymentName(''), 'success')
  assert.equal(await deploymentName('gpt-4.1-mini-ml-eval'), 'success')
  assert.notEqual(await deploymentName('-bad'), 'success')
  assert.equal(await teams('platform-eng, @CoolEngOrg/payments'), 'success')
  assert.notEqual(await teams('Platform Eng'), 'success')
  assert.equal(await repositories('payments-api, CoolEngOrg/ml-pipelines'), 'success')
  assert.notEqual(await repositories('OtherOrg/app'), 'success')
})

test('no-secrets', async () => {
  assert.equal(await noSecrets('rotate the key every 90 days'), 'success')
  assert.notEqual(await noSecrets('token: ghp_abcdefghijklmnopqrstuvwxyz0123'), 'success')
})
