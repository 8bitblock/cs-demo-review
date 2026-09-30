import { describe, expect, it } from 'vitest';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { SignalCoverage } from '../src/SignalCoverage';
import { ReviewMeasurements } from '../src/ReviewMeasurements';
import { verdictClass } from '../src/format';

describe('measured signal presentation', () => {
  it('keeps proxy sample counts separate from detector samples', () => {
    const html = renderToStaticMarkup(createElement(SignalCoverage, { capabilities: [
      { signal: 'reaction', status: 'measured', measuredSamples: 48, samples: 0, reason: 'A network proxy only.', basis: 'Spotting <state>' },
      { signal: 'recoil', status: 'limited', measuredSamples: 7, samples: 3, reason: 'Three eligible sprays.' },
    ] }));
    expect(html).toContain('Measured');
    expect(html).toContain('<strong>48</strong> measured observations');
    expect(html).toContain('<strong>0</strong> detector-eligible observations');
    expect(html).toContain('Limited sample');
    expect(html).toContain('<strong>3</strong> detector-eligible observations');
    expect(html).toContain('Spotting &lt;state&gt;');
    expect(html).not.toContain('Low concern');
  });

  it('shows measured signals and provenance without requiring an expansion', () => {
    const html = renderToStaticMarkup(createElement(ReviewMeasurements, { metrics: [
      { signal: 'shot-direction', label: 'Impact separation', value: 0.126, unit: '°', samples: 70, provenance: 'Impact-derived; not a native trajectory.' },
      { signal: 'reaction', label: 'Spotting to shot', value: 31.25, unit: 'ms', samples: 12, provenance: 'Network spotting' },
      { signal: 'recoil', label: 'Recoil cancellation residual', value: 0.035, unit: '°', samples: 9, provenance: 'Shot-native fields' },
      { label: 'Recorded sample interval', value: 15.625, unit: 'ms', samples: 300 },
    ] }));
    expect(html).not.toContain('<details');
    expect(html).toContain('Visibility reaction');
    expect(html).toContain('Recoil compensation');
    expect(html).toContain('Shot direction');
    expect(html).toContain('Recorded aim and shooting');
    expect(html).toContain('31.25');
    expect(html).toContain('0.035');
    expect(html).toContain('Impact-derived; not a native trajectory.');
    expect(html).toContain('70 observations');
  });

  it('does not colour a limited review as low concern or substitute missing values', () => {
    expect(verdictClass('Reviewed with limits')).toBe('measured');
    const html = renderToStaticMarkup(createElement(ReviewMeasurements, { metrics: [{ label: 'Unavailable observation', value: Number.NaN, unit: 'ms', samples: 0 }] }));
    expect(html).toContain('—');
    expect(html).not.toContain('NaN');
  });
});
