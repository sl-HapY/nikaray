import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:arman_vpn/app_settings.dart';
import 'package:arman_vpn/core/core_api.dart';
import 'package:arman_vpn/core/core_controller.dart';
import 'package:arman_vpn/core/models.dart';
import 'package:arman_vpn/l10n/app_localizations.dart';
import 'package:arman_vpn/ui/mobile_shell.dart';
import 'package:arman_vpn/ui/overview_page.dart';

void main() {
  test('CoreStatus parses xray engine_mode and active profile', () {
    final s = CoreStatus.fromJson({
      'engine_mode': 'xray',
      'xray': {
        'selection_mode': 'auto',
        'auto_failover': true,
        'failover_scope': 'subscription',
        'profile_count': 3,
        'subscription_count': 1,
        'active_profile': {
          'id': 'p1',
          'name': 'Reality Node',
          'protocol': 'vless',
          'address': '1.2.3.4',
          'port': 443,
          'network': 'tcp',
          'security': 'reality',
          'delay_ms': 64,
        },
      },
      'state': {'phase': 'off', 'message': 'Proxy off'},
      'accounts': <String, Object?>{},
      'dashboard': <String, Object?>{},
    });

    expect(s.isXrayMode, isTrue);
    expect(s.xray, isNotNull);
    expect(s.xray!.isAutoMode, isTrue);
    expect(s.xray!.autoFailover, isTrue);
    expect(s.xray!.failoverScope, 'subscription');
    expect(s.xray!.profileCount, 3);
    expect(s.xray!.subscriptionCount, 1);
    expect(s.xray!.activeProfile?.name, 'Reality Node');
    expect(s.xray!.activeProfile?.delayMs, 64);
  });

  testWidgets('Home displays top VPN/Xray tabs and Xray active card', (
    tester,
  ) async {
    final settings = AppSettings();
    final ctrl = CoreController(() async => throw UnimplementedError());
    ctrl.status = CoreStatus.fromJson({
      'engine_mode': 'xray',
      'xray': {
        'profile_count': 2,
        'subscription_count': 1,
        'active_profile': {
          'id': 'p1',
          'name': 'WS-TLS-DE',
          'protocol': 'vless',
          'address': 'de.example.com',
          'port': 443,
          'network': 'ws',
          'security': 'tls',
          'delay_ms': 95,
        },
      },
      'state': {'phase': 'off', 'message': 'Proxy off'},
      'accounts': <String, Object?>{},
      'dashboard': <String, Object?>{},
    });

    var openedConfigs = false;
    await tester.pumpWidget(
      MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: const [Locale('en'), Locale('fa')],
        home: Scaffold(
          body: OverviewPage(
            core: ctrl,
            settings: settings,
            phone: true,
            onOpenXrayConfigs: () => openedConfigs = true,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Mozilla'), findsOneWidget);
    expect(find.text('Xray'), findsOneWidget);
    expect(find.text('WS-TLS-DE'), findsOneWidget);
    expect(find.text('Connection mode'), findsOneWidget);
    expect(find.text('Selected config'), findsOneWidget);

    await tester.tap(find.text('WS-TLS-DE'));
    await tester.pump();
    expect(openedConfigs, isTrue);
  });

  testWidgets('Mobile Settings separates VPN and Xray tabs with shared general', (
    tester,
  ) async {
    final settings = AppSettings();
    final ctrl = CoreController(
      () async => CoreApi(
        CoreEndpoint(Uri.http('127.0.0.1:10820', '/'), 't'),
      ),
    );
    ctrl.status = CoreStatus.fromJson({
      'engine_mode': 'vpn',
      'state': {'phase': 'off', 'message': 'Proxy off'},
      'accounts': <String, Object?>{},
      'dashboard': <String, Object?>{},
    });

    await tester.pumpWidget(
      MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: const [Locale('en'), Locale('fa')],
        home: MobileShell(core: ctrl, settings: settings, banner: null),
      ),
    );
    await tester.pumpAndSettle();

    // Go to bottom Settings tab.
    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();

    // Common settings visible on VPN tab (group header + General row).
    expect(find.text('General'), findsNWidgets(2));
    expect(find.text('Routing'), findsOneWidget);
    expect(find.text('DNS'), findsOneWidget);
    // VPN-specific visible on VPN tab, Xray-specific hidden.
    expect(find.text('Servers'), findsOneWidget);
    expect(find.text('Xray Configs'), findsNothing);

    // Switch Settings top tab to Xray.
    await tester.tap(find.text('Xray').last);
    await tester.pumpAndSettle();

    // Common settings remain shared & visible; Xray-specific shown; VPN hidden.
    expect(find.text('General'), findsNWidgets(2));
    expect(find.text('Routing'), findsOneWidget);
    expect(find.text('DNS'), findsOneWidget);
    expect(find.text('Xray Configs'), findsOneWidget);
    expect(find.text('Xray Engine'), findsOneWidget);
    expect(find.text('Servers'), findsNothing);
  });

  test('CoreController.setOn(true) in Xray auto mode invokes turnOn after picking best', () async {
    bool pickedBest = false;
    bool turnedOn = false;
    final fakeApi = _MockAutoXrayApi(onPickBest: () => pickedBest = true);
    final ctrl = CoreController(
      () async => fakeApi,
      turnOn: (on) async {
        turnedOn = on;
      },
    );
    await ctrl.refresh();
    await ctrl.setOn(true);
    expect(pickedBest, isTrue);
    expect(turnedOn, isTrue);
    ctrl.dispose();
  });
}

class _MockAutoXrayApi extends CoreApi {
  _MockAutoXrayApi({required this.onPickBest})
    : super(CoreEndpoint(Uri.http('127.0.0.1:10820', '/'), 't'));

  final VoidCallback onPickBest;
  final bool _on = false;

  @override
  Future<CoreStatus> status() async => CoreStatus.fromJson({
    'engine_mode': 'xray',
    'xray': {
      'selection_mode': 'auto',
      'auto_failover': true,
      'failover_scope': 'all',
      'profile_count': 2,
      'subscription_count': 0,
    },
    'state': {'phase': _on ? 'running' : 'off', 'message': 'ok'},
    'accounts': <String, Object?>{},
    'dashboard': <String, Object?>{},
  });

  @override
  Future<AccountList> accounts() async => AccountList.fromJson(const {});

  @override
  Future<Map<String, Object?>> connectBestXray({
    List<String>? ids,
    bool connect = true,
  }) async {
    onPickBest();
    return {'active_profile_id': 'p1', 'delay_ms': 45};
  }
}
