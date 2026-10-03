import { createContext, useContext } from 'react';
import type {
  APIConnectionStatus,
	AllianceTargetViewV2,
	AllianceTargetQueryV2,
	AllianceTargetAttackPreviewRequest,
	AllianceTargetAttackPreviewV2,
	ApplicationUpdateV2,
	BuildingTargetCaptureRequest,
	BuildingTargetCaptureResponse,
  CatalogManifest,
  CatalogResponse,
  ConfigurationSnapshot,
	EquipmentOptimizeRequest,
	EquipmentOptimizeResponse,
  GameStateV2,
	IntentReceipt,
	PlayerHistoryRetentionApplyV1,
	PlayerHistoryRetentionV1,
	RuntimeDiagnosticsV2,
  SubmitIntentOptions,
} from './Contracts';

export interface APIContextValue {
  connectionStatus: APIConnectionStatus;
  state: GameStateV2 | null;
  catalogs: CatalogManifest | null;
  configuration: ConfigurationSnapshot | null;
	applicationUpdate: ApplicationUpdateV2 | null;
	diagnostics: RuntimeDiagnosticsV2 | null;
  operations: Record<string, IntentReceipt>;
  error: string | null;
  refreshState: () => Promise<void>;
  refreshCatalogs: () => Promise<void>;
  refreshConfiguration: () => Promise<void>;
  /** Loads and accepts the latest configuration snapshot; rejects on failure (draft sessions need the snapshot). */
  loadLatestConfiguration: () => Promise<ConfigurationSnapshot>;
	refreshApplicationUpdate: () => Promise<void>;
	refreshDiagnostics: () => Promise<void>;
  getCatalog: <T extends Record<string, unknown>>(name: string) => Promise<CatalogResponse<T>>;
  localize: (keys: string[]) => Promise<Record<string, string>>;
	getAllianceTargets: (input?: AllianceTargetQueryV2) => Promise<AllianceTargetViewV2>;
	previewAllianceTargetAttack: (input: AllianceTargetAttackPreviewRequest) => Promise<AllianceTargetAttackPreviewV2>;
	optimizeEquipment: (input: EquipmentOptimizeRequest) => Promise<EquipmentOptimizeResponse>;
	captureBuildingTarget: (input: BuildingTargetCaptureRequest) => Promise<BuildingTargetCaptureResponse>;
  submitIntent: (
    name: string,
    argumentsValue?: Record<string, unknown>,
    options?: SubmitIntentOptions,
  ) => Promise<IntentReceipt>;
  cancelOperation: (id: string) => Promise<void>;
  updateConfiguration: (
    section: string,
    value: unknown,
    options?: ConfigurationUpdateOptions,
  ) => Promise<ConfigurationSnapshot>;
	getPlayerHistoryRetention: () => Promise<PlayerHistoryRetentionV1>;
	applyPlayerHistoryRetention: (
		retention: string,
		recordingIntervalSeconds: number,
		expectedRevision: number,
		expectedConfigured: string,
		expectedRecordingIntervalSeconds: number,
	) => Promise<PlayerHistoryRetentionApplyV1>;
}

export type ConfigurationUpdateOptions =
	| { expectedValue: unknown; expectedRevision?: never; conflictShownByEditor?: boolean }
	| { expectedRevision: number; expectedValue?: never; conflictShownByEditor?: boolean };

export const APIContext = createContext<APIContextValue | undefined>(undefined);

export function useCitadelAPI(): APIContextValue {
  const context = useContext(APIContext);
  if (!context) throw new Error('useCitadelAPI must be used within APIProvider');
  return context;
}
