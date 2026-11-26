import 'package:material_ui/material_ui.dart';
import 'package:flutter/services.dart';

import '../core/core_controller.dart';
import '../core/models.dart';
import '../l10n/app_localizations.dart';
import 'widgets.dart';

/// Opens a dialog to import URIs (vless/vmess/trojan/ss/socks/http),
/// base64 bundles, or raw Xray JSON configs. Returns the number of added profiles.
Future<int?> showXrayImportDialog(
  BuildContext context,
  CoreController core,
) async {
  final ctrl = TextEditingController();
  try {
    return await showDialog<int>(
      context: context,
      builder: (ctx) {
        final l = AppLocalizations.of(ctx);
        var busy = false;
        return StatefulBuilder(
          builder: (ctx, setState) => AlertDialog(
            title: Text(l.xrayImportUriJson),
            content: SizedBox(
              width: 440,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  TextField(
                    controller: ctrl,
                    maxLines: 7,
                    minLines: 4,
                    textDirection: TextDirection.ltr,
                    decoration: InputDecoration(
                      labelText: l.xrayImportLabel,
                      hintText: l.xrayImportHint,
                      border: const OutlineInputBorder(),
                    ),
                  ),
                  const SizedBox(height: 8),
                  Align(
                    alignment: AlignmentDirectional.centerStart,
                    child: TextButton.icon(
                      onPressed: busy
                          ? null
                          : () async {
                              final data = await Clipboard.getData(
                                'text/plain',
                              );
                              if (data?.text != null &&
                                  data!.text!.isNotEmpty) {
                                ctrl.text = data.text!;
                              }
                            },
                      icon: const Icon(Icons.content_paste_rounded, size: 18),
                      label: Text(l.xrayAddFromClipboard),
                    ),
                  ),
                ],
              ),
            ),
            actions: [
              TextButton(
                onPressed: busy ? null : () => Navigator.pop(ctx),
                child: Text(l.cancel),
              ),
              FilledButton(
                onPressed: busy
                    ? null
                    : () async {
                        final raw = ctrl.text.trim();
                        if (raw.isEmpty) return;
                        setState(() => busy = true);
                        var count = 0;
                        final ok = await runShowingErrors(ctx, () async {
                          count = await core.call(
                            (api) => api.importXray(raw),
                          );
                        });
                        if (ctx.mounted) {
                          setState(() => busy = false);
                          if (ok) {
                            await core.refresh();
                            if (ctx.mounted) Navigator.pop(ctx, count);
                          }
                        }
                      },
                child: busy
                    ? const SizedBox.square(
                        dimension: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : Text(l.xrayImportButton),
              ),
            ],
          ),
        );
      },
    );
  } finally {
    ctrl.dispose();
  }
}

/// Opens a dialog to add or edit an Xray subscription.
Future<bool?> showXraySubscriptionDialog(
  BuildContext context,
  CoreController core, {
  XraySubscription? existing,
}) async {
  final nameCtrl = TextEditingController(text: existing?.name ?? '');
  final urlCtrl = TextEditingController(text: existing?.url ?? '');
  var autoUpdate = existing?.autoUpdate ?? true;
  try {
    return await showDialog<bool>(
      context: context,
      builder: (ctx) {
        final l = AppLocalizations.of(ctx);
        var busy = false;
        return StatefulBuilder(
          builder: (ctx, setState) => AlertDialog(
            title: Text(
              existing == null ? l.xrayAddSubscription : l.xraySubscriptions,
            ),
            content: SizedBox(
              width: 420,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  TextField(
                    controller: nameCtrl,
                    decoration: InputDecoration(
                      labelText: l.xraySubName,
                      border: const OutlineInputBorder(),
                    ),
                  ),
                  const SizedBox(height: 12),
                  TextField(
                    controller: urlCtrl,
                    textDirection: TextDirection.ltr,
                    decoration: InputDecoration(
                      labelText: l.xraySubUrl,
                      hintText: 'https://...',
                      border: const OutlineInputBorder(),
                    ),
                  ),
                  const SizedBox(height: 8),
                  SwitchListTile(
                    contentPadding: EdgeInsets.zero,
                    value: autoUpdate,
                    title: Text(l.xraySubAutoUpdate),
                    onChanged: busy
                        ? null
                        : (v) => setState(() => autoUpdate = v),
                  ),
                ],
              ),
            ),
            actions: [
              TextButton(
                onPressed: busy ? null : () => Navigator.pop(ctx, false),
                child: Text(l.cancel),
              ),
              FilledButton(
                onPressed: busy
                    ? null
                    : () async {
                        final url = urlCtrl.text.trim();
                        if (url.isEmpty) return;
                        final name = nameCtrl.text.trim();
                        setState(() => busy = true);
                        final ok = await runShowingErrors(ctx, () async {
                          await core.call((api) async {
                            if (existing == null) {
                              await api.addXraySubscription(
                                name: name,
                                url: url,
                                autoUpdate: autoUpdate,
                              );
                            } else {
                              await api.updateXraySubscription(
                                existing.id,
                                name: name,
                                url: url,
                                autoUpdate: autoUpdate,
                              );
                            }
                          });
                        });
                        if (ctx.mounted) {
                          setState(() => busy = false);
                          if (ok) {
                            await core.refresh();
                            if (ctx.mounted) Navigator.pop(ctx, true);
                          }
                        }
                      },
                child: busy
                    ? const SizedBox.square(
                        dimension: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : Text(l.save),
              ),
            ],
          ),
        );
      },
    );
  } finally {
    nameCtrl.dispose();
    urlCtrl.dispose();
  }
}

/// Opens a full manual config editor dialog supporting VLESS, VMess, Trojan,
/// Shadowsocks, SOCKS, and HTTP across all Xray transports and TLS/Reality options.
Future<bool?> showXrayManualConfigDialog(
  BuildContext context,
  CoreController core, {
  XrayProfile? existing,
}) async {
  var protocol = (existing?.protocol.isNotEmpty == true
      ? existing!.protocol
      : 'vless');
  if (!{
    'vless',
    'vmess',
    'trojan',
    'shadowsocks',
    'hysteria2',
    'socks',
    'http',
  }.contains(protocol)) {
    protocol = 'vless';
  }
  var network = (existing?.network.isNotEmpty == true
      ? existing!.network
      : 'tcp');
  var security = (existing?.security.isNotEmpty == true
      ? existing!.security
      : 'none');
  var fingerprint = existing?.fingerprint ?? '';
  var allowInsecure = existing?.allowInsecure ?? false;

  final nameCtrl = TextEditingController(text: existing?.name ?? '');
  final addrCtrl = TextEditingController(text: existing?.address ?? '');
  final portCtrl = TextEditingController(
    text: (existing?.port ?? 443).toString(),
  );
  final uuidCtrl = TextEditingController(text: existing?.uuid ?? '');
  final passCtrl = TextEditingController(text: existing?.password ?? '');
  final userCtrl = TextEditingController(text: existing?.username ?? '');
  final methodCtrl = TextEditingController(
    text: existing?.method.isNotEmpty == true
        ? existing!.method
        : 'aes-256-gcm',
  );
  final flowCtrl = TextEditingController(text: existing?.flow ?? '');
  final sniCtrl = TextEditingController(text: existing?.sni ?? '');
  final hostCtrl = TextEditingController(text: existing?.host ?? '');
  final pathCtrl = TextEditingController(text: existing?.path ?? '');
  final serviceCtrl = TextEditingController(text: existing?.serviceName ?? '');
  final modeCtrl = TextEditingController(text: existing?.mode ?? '');
  final alpnCtrl = TextEditingController(
    text: existing?.alpn.join(',') ?? '',
  );
  final pbkCtrl = TextEditingController(text: existing?.publicKey ?? '');
  final sidCtrl = TextEditingController(text: existing?.shortId ?? '');
  final spxCtrl = TextEditingController(text: existing?.spiderX ?? '');

  try {
    return await showDialog<bool>(
      context: context,
      builder: (ctx) {
        final l = AppLocalizations.of(ctx);
        var busy = false;
        return StatefulBuilder(
          builder: (ctx, setState) {
            Widget field(
              TextEditingController c,
              String label, {
              bool ltr = true,
              TextInputType? keyboard,
            }) => Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: TextField(
                controller: c,
                keyboardType: keyboard,
                textDirection: ltr ? TextDirection.ltr : null,
                decoration: InputDecoration(
                  isDense: true,
                  labelText: label,
                  border: const OutlineInputBorder(),
                ),
              ),
            );

            return AlertDialog(
              title: Text(
                existing == null ? l.xrayCreateManual : l.xrayEditConfig,
              ),
              content: SizedBox(
                width: 460,
                child: SingleChildScrollView(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      DropdownButtonFormField<String>(
                        initialValue: protocol,
                        decoration: InputDecoration(
                          isDense: true,
                          labelText: l.xrayProtocol,
                          border: const OutlineInputBorder(),
                        ),
                        items: const [
                          DropdownMenuItem(value: 'vless', child: Text('VLESS')),
                          DropdownMenuItem(value: 'vmess', child: Text('VMess')),
                          DropdownMenuItem(value: 'trojan', child: Text('Trojan')),
                          DropdownMenuItem(
                            value: 'shadowsocks',
                            child: Text('Shadowsocks'),
                          ),
                          DropdownMenuItem(
                            value: 'hysteria2',
                            child: Text('Hysteria2'),
                          ),
                          DropdownMenuItem(value: 'socks', child: Text('SOCKS5')),
                          DropdownMenuItem(value: 'http', child: Text('HTTP')),
                        ],
                        onChanged: busy
                            ? null
                            : (v) => setState(() => protocol = v ?? 'vless'),
                      ),
                      const SizedBox(height: 10),
                      field(nameCtrl, l.xrayRemark, ltr: false),
                      Row(
                        children: [
                          Expanded(flex: 3, child: field(addrCtrl, l.xrayAddress)),
                          const SizedBox(width: 8),
                          Expanded(
                            flex: 2,
                            child: field(
                              portCtrl,
                              l.xrayPort,
                              keyboard: TextInputType.number,
                            ),
                          ),
                        ],
                      ),
                      if (protocol == 'vless' || protocol == 'vmess')
                        field(uuidCtrl, l.xrayUuid),
                      if (protocol == 'trojan' ||
                          protocol == 'shadowsocks' ||
                          protocol == 'hysteria2' ||
                          protocol == 'socks' ||
                          protocol == 'http') ...[
                        if (protocol == 'socks' || protocol == 'http')
                          field(userCtrl, l.xrayUsername),
                        if (protocol == 'shadowsocks')
                          field(methodCtrl, l.xrayMethod),
                        field(passCtrl, l.xrayPassword),
                      ],
                      if (protocol == 'vless' || protocol == 'trojan')
                        field(flowCtrl, l.xrayFlow),
                      DropdownButtonFormField<String>(
                        initialValue: {
                          'tcp',
                          'ws',
                          'grpc',
                          'httpupgrade',
                          'xhttp',
                          'kcp',
                          'h2',
                        }.contains(network)
                            ? network
                            : 'tcp',
                        decoration: InputDecoration(
                          isDense: true,
                          labelText: l.xrayNetwork,
                          border: const OutlineInputBorder(),
                        ),
                        items: const [
                          DropdownMenuItem(value: 'tcp', child: Text('TCP / RAW')),
                          DropdownMenuItem(value: 'ws', child: Text('WebSocket (WS)')),
                          DropdownMenuItem(value: 'grpc', child: Text('gRPC')),
                          DropdownMenuItem(
                            value: 'httpupgrade',
                            child: Text('HTTPUpgrade'),
                          ),
                          DropdownMenuItem(value: 'xhttp', child: Text('XHTTP / SplitHTTP')),
                          DropdownMenuItem(value: 'kcp', child: Text('mKCP')),
                          DropdownMenuItem(value: 'h2', child: Text('HTTP/2 (h2)')),
                        ],
                        onChanged: busy
                            ? null
                            : (v) => setState(() => network = v ?? 'tcp'),
                      ),
                      const SizedBox(height: 10),
                      if (network == 'grpc') ...[
                        field(serviceCtrl, l.xrayServiceName),
                        field(modeCtrl, l.xrayMode),
                      ] else if (network != 'tcp') ...[
                        field(hostCtrl, l.xrayHost),
                        field(pathCtrl, l.xrayPath),
                        if (network == 'xhttp') field(modeCtrl, l.xrayMode),
                      ],
                      DropdownButtonFormField<String>(
                        initialValue: {
                          'none',
                          'tls',
                          'reality',
                        }.contains(security)
                            ? security
                            : 'none',
                        decoration: InputDecoration(
                          isDense: true,
                          labelText: l.xraySecurity,
                          border: const OutlineInputBorder(),
                        ),
                        items: const [
                          DropdownMenuItem(value: 'none', child: Text('None')),
                          DropdownMenuItem(value: 'tls', child: Text('TLS')),
                          DropdownMenuItem(
                            value: 'reality',
                            child: Text('Reality'),
                          ),
                        ],
                        onChanged: busy
                            ? null
                            : (v) => setState(() => security = v ?? 'none'),
                      ),
                      const SizedBox(height: 10),
                      if (security == 'tls' || security == 'reality') ...[
                        field(sniCtrl, l.xraySni),
                        DropdownButtonFormField<String>(
                          initialValue: {
                            '',
                            'chrome',
                            'firefox',
                            'safari',
                            'ios',
                            'android',
                            'edge',
                            'random',
                            'randomized',
                          }.contains(fingerprint)
                              ? fingerprint
                              : '',
                          decoration: InputDecoration(
                            isDense: true,
                            labelText: l.xrayFingerprint,
                            border: const OutlineInputBorder(),
                          ),
                          items: const [
                            DropdownMenuItem(value: '', child: Text('Default')),
                            DropdownMenuItem(value: 'chrome', child: Text('chrome')),
                            DropdownMenuItem(value: 'firefox', child: Text('firefox')),
                            DropdownMenuItem(value: 'safari', child: Text('safari')),
                            DropdownMenuItem(value: 'ios', child: Text('ios')),
                            DropdownMenuItem(value: 'android', child: Text('android')),
                            DropdownMenuItem(value: 'edge', child: Text('edge')),
                            DropdownMenuItem(value: 'random', child: Text('random')),
                            DropdownMenuItem(
                              value: 'randomized',
                              child: Text('randomized'),
                            ),
                          ],
                          onChanged: busy
                              ? null
                              : (v) => setState(() => fingerprint = v ?? ''),
                        ),
                        const SizedBox(height: 10),
                      ],
                      if (security == 'tls') ...[
                        field(alpnCtrl, l.xrayAlpn),
                        SwitchListTile(
                          contentPadding: EdgeInsets.zero,
                          value: allowInsecure,
                          title: Text(l.xrayAllowInsecure),
                          onChanged: busy
                              ? null
                              : (v) => setState(() => allowInsecure = v),
                        ),
                      ],
                      if (security == 'reality') ...[
                        field(pbkCtrl, l.xrayPublicKey),
                        field(sidCtrl, l.xrayShortId),
                        field(spxCtrl, l.xraySpiderX),
                      ],
                    ],
                  ),
                ),
              ),
              actions: [
                TextButton(
                  onPressed: busy ? null : () => Navigator.pop(ctx, false),
                  child: Text(l.cancel),
                ),
                FilledButton(
                  onPressed: busy
                      ? null
                      : () async {
                          final addr = addrCtrl.text.trim();
                          final port = int.tryParse(portCtrl.text.trim()) ?? 0;
                          if (addr.isEmpty || port <= 0 || port > 65535) return;
                          final profile = XrayProfile(
                            id: existing?.id ?? '',
                            name: nameCtrl.text.trim().isEmpty
                                ? '$addr:$port'
                                : nameCtrl.text.trim(),
                            protocol: protocol,
                            address: addr,
                            port: port,
                            subscriptionId: existing?.subscriptionId ?? '',
                            uuid: uuidCtrl.text.trim(),
                            password: passCtrl.text.trim(),
                            username: userCtrl.text.trim(),
                            method: methodCtrl.text.trim(),
                            flow: flowCtrl.text.trim(),
                            network: network,
                            host: hostCtrl.text.trim(),
                            path: pathCtrl.text.trim(),
                            serviceName: serviceCtrl.text.trim(),
                            mode: modeCtrl.text.trim(),
                            security: security,
                            sni: sniCtrl.text.trim(),
                            alpn: [
                              for (final part in alpnCtrl.text.split(','))
                                if (part.trim().isNotEmpty) part.trim(),
                            ],
                            fingerprint: fingerprint,
                            allowInsecure: allowInsecure,
                            publicKey: pbkCtrl.text.trim(),
                            shortId: sidCtrl.text.trim(),
                            spiderX: spxCtrl.text.trim(),
                          );
                          setState(() => busy = true);
                          final ok = await runShowingErrors(ctx, () async {
                            await core.call(
                              (api) => api.upsertXrayProfile(profile),
                            );
                          });
                          if (ctx.mounted) {
                            setState(() => busy = false);
                            if (ok) {
                              await core.refresh();
                              if (ctx.mounted) Navigator.pop(ctx, true);
                            }
                          }
                        },
                  child: busy
                      ? const SizedBox.square(
                          dimension: 16,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : Text(l.save),
                ),
              ],
            );
          },
        );
      },
    );
  } finally {
    for (final c in [
      nameCtrl,
      addrCtrl,
      portCtrl,
      uuidCtrl,
      passCtrl,
      userCtrl,
      methodCtrl,
      flowCtrl,
      sniCtrl,
      hostCtrl,
      pathCtrl,
      serviceCtrl,
      modeCtrl,
      alpnCtrl,
      pbkCtrl,
      sidCtrl,
      spxCtrl,
    ]) {
      c.dispose();
    }
  }
}
