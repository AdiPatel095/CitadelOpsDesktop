import { expect, type Page } from '@playwright/test';

export async function scanTypography(page: Page) {
  const failures = await page.evaluate(() => {
    const failures: string[] = [];
    const sizes = [12,13,14,16,20,24,32,48,64];
    const weights = [400,500,600,700];
    function scan(root: Document | ShadowRoot) {
      for (const element of root.querySelectorAll('*')) {
        if (element.shadowRoot) scan(element.shadowRoot);
        if (['SCRIPT','STYLE','OPTION'].includes(element.tagName)) continue;
        const text = [...element.childNodes].some(n=>n.nodeType===Node.TEXT_NODE && n.textContent?.trim());
        if (!text) continue;
        const style = getComputedStyle(element);
        if (style.visibility!=='visible' || style.display==='none' || !element.getClientRects().length) continue;
        const range = document.createRange(); range.selectNodeContents(element);
        if (![...range.getClientRects()].some(r=>r.width>0 && r.height>0)) continue;
        const size=parseFloat(style.fontSize), weight=Number(style.fontWeight);
        const label=`${element.tagName}.${element.getAttribute('class') ?? ''}: ${element.textContent?.trim().slice(0,45)}`;
        if (!sizes.includes(size)) failures.push(`${label}: size ${size}`);
        if (!weights.includes(weight)) failures.push(`${label}: weight ${weight}`);
        const eyebrow = /eyebrow/.test(element.getAttribute('class') ?? '') || /eyebrow/.test(element.parentElement?.getAttribute('class') ?? '');
        if (style.textTransform==='uppercase' && !eyebrow) failures.push(`${label}: uppercase outside eyebrow`);
        if (element.matches(':lang(ar),:lang(ja),:lang(ko),:lang(zh)') && !['normal','0px'].includes(style.letterSpacing)) failures.push(`${label}: locale tracking ${style.letterSpacing}`);
      }
    }
    scan(document);
    return failures;
  });
  expect(failures, 'CIT-63 R4/R5/R13 rendered typography').toEqual([]);
  for (const metric of await page.locator('.ui-metric-value:visible').all()) {
    expect(await metric.evaluate(element=>getComputedStyle(element).fontWeight), 'metric values retain the 700 role override').toBe('700');
  }
}

export async function checkLocaleLeading(page: Page, locale: string) {
  // Test the role tokens in a genuine language container. Fallback messages may
  // keep lang=en and inherit their parent's computed leading in the full UI.
  const metrics = await page.evaluate(locale => {
    const container=document.createElement('div');container.lang=locale;
    document.body.append(container);
    const results=[12,13,14,16].map(size=>{
      const element=document.createElement('span');element.textContent='Type scale';
      element.style.fontSize=`var(--font-size-${size})`;
      element.style.lineHeight=`var(--line-height-${size})`;
      element.style.letterSpacing='var(--tracking-eyebrow)';
      element.style.textTransform='var(--eyebrow-case)';container.append(element);
      const style=getComputedStyle(element);
      return {size:parseFloat(style.fontSize),line:parseFloat(style.lineHeight),tracking:parseFloat(style.letterSpacing) || 0,casing:style.textTransform};
    });container.remove();return results;
  },locale);
  expect(metrics).toEqual([12,13,14,16].map((size,index)=>({size,line:[20,22,24,28][index],tracking:0,casing:'none'})));
}

export async function checkDialogWrapping(page: Page) {
  // Real shared classes are used by all 18 titles. Exercise a short title, a
  // Russian multiword title and an unbroken name at the narrow viewport boundary.
  const results = await page.evaluate(() => {
    const wrapper=document.createElement('div');
    wrapper.style.cssText='position:fixed;top:0;left:0;width:180px;z-index:99999';
    document.body.append(wrapper);
    const results=[];
    for(const className of ['picker-modal-title-text','scheduler-modal-title-text']) {
      const title=document.createElement('div');title.className=className;wrapper.append(title);
      title.textContent='Settings'; const short=title.getBoundingClientRect().height;
      title.textContent='Настройки автоматического строительства замка';
      const style=getComputedStyle(title), long=title.getBoundingClientRect().height;
      title.textContent='VeryLongCastleName'.repeat(8);
      results.push({className,short,long,wordBreak:style.overflowWrap,whiteSpace:style.whiteSpace,ellipsis:style.textOverflow,clamp:style.webkitLineClamp,overflow:title.scrollWidth-title.clientWidth});
      title.remove();
    }
    wrapper.remove();return results;
  });
  for (const result of results) {
    expect(result.wordBreak).toBe('anywhere');
    expect(result.whiteSpace).toBe('normal');
    expect(result.ellipsis).not.toBe('ellipsis');
    expect(result.clamp).toBe('none');
    expect(result.short).toBe(28);
    expect(result.long).toBeGreaterThan(result.short);
    expect(result.overflow).toBeLessThanOrEqual(1);
  }
}
