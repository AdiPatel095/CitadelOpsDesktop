import type { VisualCase } from './cases';
import { readFile } from 'node:fs/promises';
import { expect, type Page, type Request } from '@playwright/test';

export async function prepare(page: Page, theme: 'dark' | 'light', states?: VisualCase['states']) {
  const intercepted = new WeakSet<Request>();
  const escapes: string[] = [];
  const unhandled: string[] = [];
  const placeholder = await readFile(new URL('./fixtures/placeholder.png', import.meta.url));
  const context = page.context();
  const verifyRequest = (request: Request) => {
    if (new URL(request.url()).hostname !== '127.0.0.1' && !intercepted.has(request)) escapes.push(request.url());
  };
  context.on('requestfinished', verifyRequest);
  context.on('requestfailed', verifyRequest);
  page.on('console', (message) => {
    if (message.text().includes('[mock] unhandled runtime path')) unhandled.push(message.text());
  });
  await context.route('**/*', async (route) => {
    const request = route.request();
    if (new URL(request.url()).hostname === '127.0.0.1') return route.continue();
    intercepted.add(request);
    if (request.resourceType() === 'image') {
      await route.fulfill({ status: 200, contentType: 'image/png', body: placeholder });
    } else {
      await route.abort('blockedbyclient');
    }
  });
  await context.routeWebSocket('**/*', (socket) => {
    if (new URL(socket.url()).hostname === '127.0.0.1') socket.connectToServer();
    else socket.close();
  });
  await page.addInitScript(({ theme, states }) => {
    localStorage.setItem('theme', theme);
    if (states) localStorage.setItem('citadelops.visualStates', JSON.stringify(states)); else localStorage.removeItem('citadelops.visualStates');
    let seed = 61;
    Math.random = () => {
      seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0;
      return seed / 4294967296;
    };
  }, { theme, states });
  await page.clock.setFixedTime(new Date('2026-09-29T12:00:00Z'));
  await page.goto('/?scenario=rich-account&locale=en&reset=1');
  await page.addStyleTag({ content: `
    #fixture-banner, #fixture-dock { display: none !important; }
    #root { margin-top: 0 !important; height: 100dvh !important; transform: none !important; }
  ` });
  return () => {
    expect(escapes, 'Requests escaped the interception policy').toEqual([]);
    expect(unhandled, 'Mock runtime paths must be handled').toEqual([]);
  };
}

export async function openView(page: Page, label: string, view: string) {
  if ((page.viewportSize()?.width ?? 0) < 760) {
    await page.getByRole('button', { name: 'Open workspace navigation', exact: true }).click();
  }
  await page.locator('#workspace-navigation').getByRole('button', { name: label, exact: true }).click();
  await expect(page.locator(`[data-view="${view}"]`)).toBeVisible();
}

export async function openSettings(page: Page) {
  await page.getByRole('button', { name: 'Open Auto Towers settings', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
}

export async function settle(page: Page) {
  await page.waitForLoadState('networkidle');
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(2000);
}
