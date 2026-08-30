import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { CheckCircle2, Download, RefreshCw } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import { Button, Card, Modal } from './ui';

const ignoredVersionKey = 'ignoredVersion';

const UpdateModal = () => {
	const { applicationUpdate, refreshApplicationUpdate, submitIntent } = useCitadelAPI();
	const [dismissedVersion, setDismissedVersion] = useState<string | null>(null);
	const [installing, setInstalling] = useState(false);
	const [installError, setInstallError] = useState('');
	const latestVersion = applicationUpdate?.latestVersion ?? '';
	const ignoredVersion = localStorage.getItem(ignoredVersionKey);

	useEffect(() => {
		if (!installing && applicationUpdate?.status !== 'downloading' && applicationUpdate?.status !== 'installing') return;
		const interval = window.setInterval(() => void refreshApplicationUpdate(), 300);
		return () => window.clearInterval(interval);
	}, [applicationUpdate?.status, installing, refreshApplicationUpdate]);

	if (!applicationUpdate) return null;
	if (applicationUpdate.restartRequired) {
		return createPortal(
			<div className="fixed inset-0 z-[200] flex items-center justify-center bg-bg-app text-text-main">
				<div className="relative max-w-lg p-12 text-center animate-fade-in">
					<div className="mb-8 flex justify-center">
						<span className="flex h-24 w-24 items-center justify-center rounded-full bg-success/20 text-success shadow-[0_0_40px_var(--color-success)]">
							<CheckCircle2 className="h-12 w-12" />
						</span>
					</div>
					<h1 className="mb-4 text-3xl font-bold">Update complete</h1>
					<p className="mb-8 text-lg leading-relaxed text-text-muted">
						Version <span className="font-semibold text-primary">{latestVersion}</span> is installed.
					</p>
					<Card variant="solid" className="mb-8 border-primary/30 p-6 shadow-lg">
						<div className="flex items-center justify-center gap-3 text-xl font-bold text-primary">
							<RefreshCw className="h-6 w-6" /> Restart CitadelOps
						</div>
						<p className="mt-3 text-sm text-text-muted">Close this window and reopen the application to run the new binary.</p>
					</Card>
				</div>
			</div>,
			document.body,
		);
	}

	const activelyInstalling = installing || applicationUpdate.status === 'downloading' || applicationUpdate.status === 'installing';
	const hidden = !activelyInstalling && (
		!applicationUpdate.available
		|| latestVersion === dismissedVersion
		|| latestVersion === ignoredVersion
	);
	if (hidden) return null;

	const dismiss = () => setDismissedVersion(latestVersion);
	const ignore = () => {
		localStorage.setItem(ignoredVersionKey, latestVersion);
		setDismissedVersion(latestVersion);
	};
	const install = () => {
		if (!applicationUpdate.installSupported) {
			if (applicationUpdate.downloadUrl) window.open(applicationUpdate.downloadUrl, '_blank', 'noopener,noreferrer');
			return;
		}
		setInstalling(true);
		setInstallError('');
		void submitIntent('app.update.install')
			.catch((reason) => setInstallError(reason instanceof Error ? reason.message : 'Application update failed.'))
			.finally(() => {
				setInstalling(false);
				void refreshApplicationUpdate();
			});
	};

	return (
		<Modal
			isOpen
			onClose={activelyInstalling ? () => undefined : dismiss}
			maxWidth="md"
			hideCloseButton={activelyInstalling}
			title={(
				<div className="flex flex-col items-center pt-4 text-center">
					<span className="mb-6 flex h-20 w-20 items-center justify-center rounded-full bg-primary/20 text-primary shadow-[0_0_20px_var(--color-primary-glow)]">
						<Download className="h-10 w-10" />
					</span>
					<div className="mb-4 rounded-full border border-primary/30 bg-primary/10 px-3 py-1 text-xs font-bold uppercase tracking-wider text-primary">
						{activelyInstalling ? 'Installing update' : 'Update available'}
					</div>
					<h3 className="text-2xl font-bold text-text-main">Version {latestVersion}</h3>
				</div>
			)}
			footer={!activelyInstalling ? (
				<div className="flex w-full gap-3">
					<Button variant="ghost" onClick={dismiss} className="flex-1">Later</Button>
					<Button variant="outline" onClick={ignore} className="flex-1">Ignore</Button>
					<Button variant="primary" onClick={install} className="flex-[2]" leftIcon={<Download className="h-4 w-4" />}>
						{applicationUpdate.installSupported ? 'Update now' : 'Download'}
					</Button>
				</div>
			) : undefined}
		>
			<div className="space-y-5 pb-2 text-center">
				<p className="leading-relaxed text-text-muted">
					{activelyInstalling ? applicationUpdate.stage ?? 'Preparing update...' : 'A new CitadelOps Desktop release is ready.'}
				</p>
				{activelyInstalling && (
					<div>
						<div className="mb-2 h-3 w-full overflow-hidden rounded-full bg-bg-app">
							<div className="h-full rounded-full bg-primary transition-all duration-300" style={{ width: `${applicationUpdate.progress}%` }} />
						</div>
						<p className="text-sm font-semibold text-text-muted">{applicationUpdate.progress}% complete</p>
					</div>
				)}
				{(installError || applicationUpdate.error) && (
					<div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error">
						{installError || applicationUpdate.error}
					</div>
				)}
				<a href="https://citadelops.app/" target="_blank" rel="noopener noreferrer" className="text-sm font-medium text-primary hover:text-primary-hover">
					View patch notes
				</a>
			</div>
		</Modal>
	);
};

export default UpdateModal;
