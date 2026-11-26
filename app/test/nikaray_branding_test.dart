import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:arman_vpn/app_info.dart';
import 'package:arman_vpn/app_settings.dart';
import 'package:arman_vpn/core/core_controller.dart';
import 'package:arman_vpn/l10n/app_localizations.dart';
import 'package:arman_vpn/ui/about_page.dart';
import 'package:arman_vpn/ui/app.dart';
import 'package:arman_vpn/ui/widgets.dart';

import 'widget_test.dart' show FakeApi, stillMotion;

void main() {
  testWidgets('phone home and about page show NikaRay, salist labs, and GitHub URL', (
    tester,
  ) async {
    stillMotion(tester);
    tester.view.physicalSize = const Size(420, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    final core = CoreController(
      () async => FakeApi(),
      pollInterval: const Duration(hours: 1),
    );
    await tester.pumpWidget(VpnApp(core: core, settings: AppSettings()));
    core.start();
    await tester.pumpAndSettle();

    expect(find.text('NikaRay'), findsOneWidget);
    expect(find.text('by salist labs'), findsOneWidget);
    expect(find.byType(AppLogo), findsOneWidget);

    await tester.tap(find.byIcon(Icons.info_outline_rounded));
    await tester.pumpAndSettle();

    expect(find.byType(AboutPage), findsOneWidget);
    expect(find.text('NikaRay'), findsOneWidget);
    expect(find.text('by salist labs'), findsOneWidget);
    expect(find.text('https://github.com/sl-HapY/nikaray'), findsOneWidget);
    expect(githubRepo, 'sl-HapY/nikaray');

    await tester.pumpWidget(const SizedBox());
    core.dispose();
  });

  testWidgets('welcome page shows NikaRay branding and logo', (tester) async {
    stillMotion(tester);
    final core = CoreController(
      () async => FakeApi(),
      pollInterval: const Duration(hours: 1),
    );
    final settings = AppSettings()..firstRun = true;
    await tester.pumpWidget(VpnApp(core: core, settings: settings));
    core.start();
    await tester.pumpAndSettle();

    expect(find.text('NikaRay'), findsOneWidget);
    expect(find.byType(AppLogo), findsOneWidget);
    final l = lookupAppLocalizations(const Locale('en'));
    expect(l.appTitle, 'NikaRay');

    await tester.pumpWidget(const SizedBox());
    core.dispose();
  });
}
