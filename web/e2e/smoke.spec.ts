import { expect, test } from '@playwright/test'

// PLAN §12: open → drill to applyTiered → annotations are shown.
test('drill from the module to applyTiered', async ({ page }) => {
  await page.goto('/')
  const pricing = page.locator('button[data-qt-id="github.com/acme/ledger/internal/pricing"]')
  await expect(pricing).toBeVisible()
  await expect(page.getByRole('complementary', { name: 'Inspector' })).toContainText('internal/pricing')

  await pricing.dblclick()
  await expect(page).toHaveURL(/#\/pkg\//)
  const tiered = page.locator('button[data-qt-id="github.com/acme/ledger/internal/pricing.applyTiered"]')
  await expect(tiered).toBeVisible()

  await tiered.dblclick()
  await expect(page).toHaveURL(/#\/func\//)
  await expect(page.getByRole('region', { name: /Source of/ })).toContainText('func applyTiered(')
  // The fixture's mutation results are committed (testdata/ledger/.qtldr/mutation).
  await expect(page.locator('.badge', { hasText: 'SURVIVED' }).first()).toBeVisible()
  await expect(page.locator('.badge', { hasText: 'NOT COVERED' }).first()).toBeVisible()
  await expect(page.getByRole('complementary', { name: 'Inspector' })).toContainText('Surviving mutants')

  await page.keyboard.press('Escape')
  await expect(page).toHaveURL(/#\/pkg\//)
})
