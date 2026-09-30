import type { ReviewMetric, Signal } from '../shared/types';
import { measurementValue, signalLabel } from './format';

const signalOrder: (Signal | undefined)[] = ['reaction', 'recoil', 'shot-direction', 'acquisition', 'aim-snap', undefined];

export function ReviewMeasurements({ metrics }: { metrics: ReviewMetric[] }) {
  if (!metrics.length) return null;
  return <div className="review-measurements" aria-label="Measured aim and shooting">
    <p>Recorded measurements describe the available observations. Detector coverage and repeated findings are shown separately.</p>
    {signalOrder.map(signal => {
      const group = metrics.filter(metric => metric.signal === signal);
      if (!group.length) return null;
      const sources = [...new Set(group.map(metric => metric.provenance).filter(Boolean))];
      return <section key={signal ?? 'recording'} className="review-metric-group">
        <h3>{signal ? signalLabel[signal] : 'Recorded aim and shooting'}</h3>
        <dl>{group.map((metric, index) => <div key={`${metric.label}-${index}`}><dt>{metric.label}<small>{metric.samples.toLocaleString()} observations</small></dt><dd>{measurementValue(metric.value)} <small>{metric.unit}</small></dd></div>)}</dl>
        {sources.map(source => <p className="metric-provenance" key={source}><b>Source:</b> {source}</p>)}
      </section>;
    })}
  </div>;
}
