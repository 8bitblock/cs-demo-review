import type { Capability, CapabilityStatus } from '../shared/types';
import { signalLabel } from './format';

const statusLabel: Record<CapabilityStatus, string> = {
  available: 'Assessed', limited: 'Limited sample', measured: 'Measured',
  insufficient: 'Missing observations', unsupported: 'Unavailable',
};

/** Telemetry can be useful even when it cannot support a detector or a verdict. */
export function SignalCoverage({ capabilities }: { capabilities: Capability[] }) {
  return <div className="capabilities">{capabilities.map(capability => <div className="capability" key={capability.signal}>
    <div><span className={`status-dot ${capability.status === 'available' ? 'green' : capability.status === 'limited' || capability.status === 'insufficient' ? 'amber' : 'gray'}`} /><span>{signalLabel[capability.signal]}</span><span className={`cap-status ${capability.status}`}>{statusLabel[capability.status]}</span></div>
    <div className="capability-counts">
      {capability.measuredSamples != null && <span><strong>{capability.measuredSamples.toLocaleString()}</strong> measured observations</span>}
      <span><strong>{capability.samples.toLocaleString()}</strong> detector-eligible observations</span>
    </div>
    <p>{capability.reason}</p>
    {capability.basis && <p className="capability-basis"><b>Source:</b> {capability.basis}</p>}
  </div>)}</div>;
}
