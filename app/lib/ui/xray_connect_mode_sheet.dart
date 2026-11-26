import 'package:material_ui/material_ui.dart';

import '../core/core_controller.dart';
import '../l10n/app_localizations.dart';
import 'widgets.dart';

/// Monochrome bottom sheet letting the user configure Xray connection
/// selection (`manual` vs `auto` best-ping & auto-failover) and failover scope.
Future<void> showXrayConnectModeSheet(
  BuildContext context,
  CoreController core,
) async {
  await showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    builder: (ctx) => _XrayConnectModeSheet(core: core),
  );
}

class _XrayConnectModeSheet extends StatefulWidget {
  const _XrayConnectModeSheet({required this.core});

  final CoreController core;

  @override
  State<_XrayConnectModeSheet> createState() => _XrayConnectModeSheetState();
}

class _XrayConnectModeSheetState extends State<_XrayConnectModeSheet> {
  String _selectionMode = 'manual';
  bool _autoFailover = true;
  String _scope = 'all';
  bool _busy = false;
  bool _findingBest = false;

  @override
  void initState() {
    super.initState();
    final x = widget.core.status?.xray;
    if (x != null) {
      _selectionMode = x.selectionMode;
      _autoFailover = x.autoFailover;
      _scope = x.failoverScope;
    }
  }

  Future<void> _save({
    String? selectionMode,
    bool? autoFailover,
    String? scope,
  }) async {
    setState(() => _busy = true);
    final ok = await runShowingErrors(context, () async {
      await widget.core.editSettings((cfg) {
        final xray = (cfg['xray'] is Map<String, Object?>)
            ? cfg['xray']! as Map<String, Object?>
            : (cfg['xray'] = <String, Object?>{});
        if (selectionMode != null) xray['selection_mode'] = selectionMode;
        if (autoFailover != null) xray['auto_failover'] = autoFailover;
        if (scope != null) xray['failover_scope'] = scope;
      });
    });
    if (mounted) {
      setState(() {
        _busy = false;
        if (ok) {
          if (selectionMode != null) _selectionMode = selectionMode;
          if (autoFailover != null) _autoFailover = autoFailover;
          if (scope != null) _scope = scope;
        }
      });
    }
  }

  Future<void> _connectBestNow() async {
    setState(() => _findingBest = true);
    final ok = await runShowingErrors(context, () async {
      await widget.core.connectBestXrayAndStart();
    });
    if (mounted) {
      setState(() => _findingBest = false);
      if (ok) Navigator.pop(context);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final theme = Theme.of(context);
    final cs = theme.colorScheme;

    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 16, 20, 20),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                const Icon(Icons.auto_fix_high_rounded),
                const SizedBox(width: 10),
                Text(
                  l.xrayConnectMode,
                  style: theme.textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            Card(
              margin: EdgeInsets.zero,
              child: RadioGroup<String>(
                groupValue: _selectionMode,
                onChanged: (v) {
                  if (!_busy && v != null) _save(selectionMode: v);
                },
                child: Column(
                  children: [
                    RadioListTile<String>(
                      value: 'manual',
                      title: Text(l.xrayModeManual),
                      subtitle: Text(
                        l.xrayModeManualHint,
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: cs.onSurfaceVariant,
                        ),
                      ),
                    ),
                    const Divider(height: 1),
                    RadioListTile<String>(
                      value: 'auto',
                      title: Text(l.xrayModeAuto),
                      subtitle: Text(
                        l.xrayModeAutoHint,
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: cs.onSurfaceVariant,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 12),
            Card(
              margin: EdgeInsets.zero,
              child: Column(
                children: [
                  SwitchListTile(
                    value: _autoFailover,
                    title: Text(l.xrayAutoFailover),
                    onChanged: _busy
                        ? null
                        : (v) => _save(autoFailover: v),
                  ),
                  const Divider(height: 1),
                  Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 16,
                      vertical: 10,
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Text(
                          l.xrayFailoverScope,
                          style: theme.textTheme.bodyMedium?.copyWith(
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        const SizedBox(height: 8),
                        SegmentedButton<String>(
                          segments: [
                            ButtonSegment<String>(
                              value: 'all',
                              label: Text(l.xrayScopeAll),
                            ),
                            ButtonSegment<String>(
                              value: 'subscription',
                              label: Text(l.xrayScopeSub),
                            ),
                          ],
                          selected: {_scope},
                          onSelectionChanged: _busy
                              ? null
                              : (s) => _save(scope: s.first),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 14),
            FilledButton.icon(
              onPressed: _findingBest || _busy ? null : _connectBestNow,
              icon: _findingBest
                  ? const SizedBox.square(
                      dimension: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.bolt_rounded),
              label: Text(l.xrayConnectBestNow),
            ),
          ],
        ),
      ),
    );
  }
}
