/** Official v4357 keys whose English templates mark retired effects. Never inspect viewer prose. */
const retired4357 = new Set([
  'equip_effect_description_attackBoostYardShapeshifter',
  'equip_effect_description_attackUnitAmountFlankShapeshifter',
  'equip_effect_description_charmBoost',
]);
const japaneseSignCorrections4357 = new Set([
  'equip_effect_description_wallReductionCharge',
  'equip_effect_description_gateReductionCharge',
]);
export function equipmentEffectTemplates(keys: string[], selected: Record<string,string>, canonical: Record<string,string>, locale: string, version: string) {
  const eligible = keys.filter(key => version !== '4357' || !retired4357.has(key));
  const semanticKey = eligible.find(key => canonical[key]?.trim());
  const localizationKey = eligible.find(key => selected[key]?.trim());
  const semanticTemplate = semanticKey ? canonical[semanticKey].trim() : '';
  const originalTemplate = localizationKey ? selected[localizationKey].trim() : '';
  // Verified official v4357 Japanese mistakes: English reduction is negative,
  // while these exact Japanese keys incorrectly contain +{0}%. Catalog stays intact.
  const corrected = locale === 'ja' && version === '4357' && !!localizationKey
    && japaneseSignCorrections4357.has(localizationKey)
    && semanticTemplate.includes('-{0}%') && originalTemplate.includes('+{0}%');
  return {
    semanticTemplate, localizationKey,
    effectTemplate: corrected ? originalTemplate.replace('+{0}%', '-{0}%') : originalTemplate,
    originalEffectTemplate: originalTemplate,
    displayCorrection: corrected ? {key:localizationKey,locale,version,reason:'official-sign-conflicts-with-canonical-reduction'} : undefined,
  };
}
