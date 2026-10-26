// FinalAdmin groups the administration pages for the final round of features. Each page reads the live
// view from the store and saves the whole settings object (or its own connection), following the way the
// other administration pages work. The pages take no props; the wiring engineer mounts them in Admin.tsx.
//
//   CriticalAdmin  - Critical incidents
//   FlagsAdmin     - Premise and safety flags
//   CallEntryRules - Call entry rules
//   PipAdmin       - PSC PIP connection

export { CriticalAdmin } from './FinalCritical'
export { FlagsAdmin } from './FinalFlags'
export { CallEntryRules } from './FinalCallEntry'
export { PipAdmin } from './FinalPip'
