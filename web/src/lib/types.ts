// Types mirroring api/openapi.yaml (M2/M3 subset).

export interface SetupStatus {
	setupAvailable: boolean;
}

export interface Me {
	administratorId: string;
	sessionExpiresAt: string;
}

export interface SystemInfo {
	version: string;
	architecture: string;
	clusterReachable: boolean;
	capabilities: string[];
}

export interface ApplicationSummary {
	id: string;
	name: string;
	version: number;
	phase: string;
}

export interface ApplicationList {
	items: ApplicationSummary[];
}

export interface ObservedStatus {
	phase: string;
	desiredReplicas: number;
	readyReplicas: number;
	workloadReady: boolean;
	routeReady: boolean;
	observedAt: string;
	stale: boolean;
}

export interface Application {
	id: string;
	name: string;
	version: number;
	desiredRevision: string;
	lastSuccessfulRevision: string;
	activeOperationId: string;
	status: ObservedStatus;
}

export interface OperationEvent {
	at: string;
	stage: string;
	message: string;
}

export interface Operation {
	id: string;
	applicationId: string;
	type: string;
	state: string;
	stage: string;
	recoveryOutcome: string;
	events: OperationEvent[];
}

export interface AcceptResult {
	applicationId: string;
	operationId: string;
	state: string;
	deduplicated: boolean;
}

export interface Secret {
	id: string;
	name: string;
	versions: number[];
}

export interface RegistryCredential {
	id: string;
	label: string;
	registry: string;
	username: string;
}

export interface ImageInspectResult {
	digest: string;
	architectures: string[];
	suggestedPort: number | null;
}

export interface CreateApplicationRequest {
	name: string;
	image: string;
	registryCredentialId?: string;
	containerPort: number;
	profile: string;
	replicas?: number;
	environment?: Record<string, string>;
	secretRefs?: Array<{ environmentVariable: string; secretId: string; version: number }>;
	resources?: {
		cpuRequest?: string;
		cpuLimit?: string;
		memoryRequest?: string;
		memoryLimit?: string;
	};
	health?: { readinessPath?: string };
	exposure: { mode: string; hostname?: string };
}
