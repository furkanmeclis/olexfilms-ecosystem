/**
 * Config plugin of the caller ID (TEC-1208): adds the iOS Call Directory
 * extension target to the generated Xcode project.
 *
 * iOS shows a caller's name from a list an app extension loaded earlier; the
 * extension is a second binary inside the app, with its own bundle id and
 * signing. This plugin:
 *   - copies modules/caller-id/extension/*.swift into ios/OlexCallerId/ and
 *     writes the target's Info.plist and entitlements (App Group),
 *   - adds the `OlexCallerId` app-extension target and embeds it in the app,
 *   - puts the App Group and the extension id in the app's Info.plist (the
 *     native module reads them) and the App Group in the app's entitlements,
 *   - declares the extension for EAS Build, so its provisioning profile is
 *     created with the App Group.
 *
 * Android needs no plugin: the module's AndroidManifest.xml is merged.
 *
 * Usage (app.json): ["./plugins/with-caller-id", { "groupIdentifier": "group.com.olexfilms.app" }]
 * The group is the one the widgets already use.
 */
const { withDangerousMod, withEntitlementsPlist, withInfoPlist, withXcodeProject } = require('expo/config-plugins');
const fs = require('node:fs');
const path = require('node:path');

const TARGET = 'OlexCallerId';
const EMBED_PHASE = 'Embed Foundation Extensions';
const GROUPS_KEY = 'com.apple.security.application-groups';
const EXTENSION_SOURCES = path.join('modules', 'caller-id', 'extension');

const plistHeader =
  '<?xml version="1.0" encoding="UTF-8"?>\n' +
  '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n' +
  '<plist version="1.0">\n';

const xml = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

/** The name iOS shows in Settings and next to the caller ("<name> Caller ID: ..."). */
const infoPlist = ({ displayName, groupIdentifier, version, buildNumber }) =>
  `${plistHeader}<dict>
	<key>CFBundleDisplayName</key>
	<string>${xml(displayName)}</string>
	<key>CFBundleShortVersionString</key>
	<string>${xml(version)}</string>
	<key>CFBundleVersion</key>
	<string>${xml(buildNumber)}</string>
	<key>OlexCallerIdAppGroup</key>
	<string>${xml(groupIdentifier)}</string>
	<key>NSExtension</key>
	<dict>
		<key>NSExtensionPointIdentifier</key>
		<string>com.apple.callkit.call-directory</string>
		<key>NSExtensionPrincipalClass</key>
		<string>$(PRODUCT_MODULE_NAME).CallDirectoryHandler</string>
	</dict>
</dict>
</plist>
`;

const entitlements = (groupIdentifier) =>
  `${plistHeader}<dict>
	<key>${GROUPS_KEY}</key>
	<array>
		<string>${xml(groupIdentifier)}</string>
	</array>
</dict>
</plist>
`;

function swiftSources(projectRoot) {
  const dir = path.join(projectRoot, EXTENSION_SOURCES);
  return fs
    .readdirSync(dir)
    .filter((f) => f.endsWith('.swift'))
    .sort()
    .map((f) => path.join(dir, f));
}

function resolve(config, props) {
  const appId = config.ios?.bundleIdentifier;
  if (!appId) throw new Error('with-caller-id: ios.bundleIdentifier is required');
  if (!props?.groupIdentifier) throw new Error('with-caller-id: groupIdentifier is required');
  return {
    groupIdentifier: props.groupIdentifier,
    bundleIdentifier: `${appId}.callerid`,
    displayName: props.displayName ?? config.name ?? TARGET,
    deploymentTarget: config.ios?.deploymentTarget ?? '16.4',
    version: config.ios?.version ?? config.version ?? '1.0',
    buildNumber: config.ios?.buildNumber ?? '1',
  };
}

/** EAS Build reads this list to create credentials for extension targets. */
function withEasExtension(config, { bundleIdentifier, groupIdentifier }) {
  const eas = config.extra?.eas ?? {};
  const ios = eas.build?.experimental?.ios ?? {};
  const others = (ios.appExtensions ?? []).filter((e) => e.targetName !== TARGET);
  config.extra = {
    ...config.extra,
    eas: {
      ...eas,
      build: {
        ...eas.build,
        experimental: {
          ...eas.build?.experimental,
          ios: {
            ...ios,
            appExtensions: [
              ...others,
              { targetName: TARGET, bundleIdentifier, entitlements: { [GROUPS_KEY]: [groupIdentifier] } },
            ],
          },
        },
      },
    },
  };
  return config;
}

const withTargetFiles = (config, opts) =>
  withDangerousMod(config, [
    'ios',
    (config) => {
      const dir = path.join(config.modRequest.platformProjectRoot, TARGET);
      fs.mkdirSync(dir, { recursive: true });
      for (const file of swiftSources(config.modRequest.projectRoot)) {
        fs.copyFileSync(file, path.join(dir, path.basename(file)));
      }
      fs.writeFileSync(path.join(dir, 'Info.plist'), infoPlist(opts));
      fs.writeFileSync(path.join(dir, `${TARGET}.entitlements`), entitlements(opts.groupIdentifier));
      return config;
    },
  ]);

function buildPhase(project, type, targetUuid, comment) {
  const section = project.hash.project.objects[type];
  const target = project.pbxNativeTargetSection()[targetUuid];
  const ref = target?.buildPhases?.find((p) => (!comment || p.comment === comment) && section?.[p.value]);
  return ref ? section[ref.value] : null;
}

const withTarget = (config, opts) =>
  withXcodeProject(config, (config) => {
    const project = config.modResults;
    if (project.findTargetKey(TARGET)) return config;
    const objects = project.hash.project.objects;
    const sources = swiftSources(config.modRequest.projectRoot).map((f) => path.basename(f));
    const mainTargetUuid = project.getFirstTarget().uuid;

    const settings = {
      PRODUCT_NAME: '"$(TARGET_NAME)"',
      PRODUCT_BUNDLE_IDENTIFIER: `"${opts.bundleIdentifier}"`,
      INFOPLIST_FILE: `${TARGET}/Info.plist`,
      GENERATE_INFOPLIST_FILE: '"YES"',
      INFOPLIST_KEY_NSHumanReadableCopyright: '""',
      CODE_SIGN_ENTITLEMENTS: `"${TARGET}/${TARGET}.entitlements"`,
      CURRENT_PROJECT_VERSION: `"${opts.buildNumber}"`,
      MARKETING_VERSION: `"${opts.version}"`,
      IPHONEOS_DEPLOYMENT_TARGET: `"${opts.deploymentTarget}"`,
      TARGETED_DEVICE_FAMILY: '"1,2"',
      SWIFT_VERSION: '5.0',
      APPLICATION_EXTENSION_API_ONLY: '"YES"',
      SKIP_INSTALL: '"YES"',
      ...(config.ios?.appleTeamId ? { DEVELOPMENT_TEAM: config.ios.appleTeamId } : {}),
    };
    const configurations = project.addXCConfigurationList(
      [
        { name: 'Debug', isa: 'XCBuildConfiguration', buildSettings: { ...settings, SWIFT_OPTIMIZATION_LEVEL: '"-Onone"' } },
        { name: 'Release', isa: 'XCBuildConfiguration', buildSettings: { ...settings } },
      ],
      'Release',
      `Build configuration list for PBXNativeTarget "${TARGET}"`,
    );

    const product = project.addProductFile(TARGET, {
      basename: `${TARGET}.appex`,
      group: EMBED_PHASE,
      explicitFileType: 'wrapper.app-extension',
      settings: { ATTRIBUTES: ['RemoveHeadersOnCopy'] },
      includeInIndex: 0,
      path: `${TARGET}.appex`,
      sourceTree: 'BUILT_PRODUCTS_DIR',
    });

    const target = {
      uuid: project.generateUuid(),
      pbxNativeTarget: {
        isa: 'PBXNativeTarget',
        name: TARGET,
        productName: TARGET,
        productReference: product.fileRef,
        productType: '"com.apple.product-type.app-extension"',
        buildConfigurationList: configurations.uuid,
        buildPhases: [],
        buildRules: [],
        dependencies: [],
      },
    };
    project.addToPbxNativeTargetSection(target);
    project.addToPbxProjectSection(target);
    const pbxProject = project.pbxProjectSection()[project.getFirstProject().uuid];
    pbxProject.attributes.TargetAttributes ??= {};
    pbxProject.attributes.TargetAttributes[target.uuid] = { LastSwiftMigration: 1250 };

    // The app builds the extension first...
    objects.PBXTargetDependency ??= {};
    objects.PBXContainerItemProxy ??= {};
    project.addTargetDependency(mainTargetUuid, [target.uuid]);

    // ...compiles its sources...
    project.addBuildPhase(sources, 'PBXSourcesBuildPhase', 'Sources', target.uuid, 'app_extension', '""');
    project.addBuildPhase([], 'PBXFrameworksBuildPhase', 'Frameworks', target.uuid, 'app_extension', '""');

    // ...and embeds it (the phase is shared with the widget extension).
    if (!buildPhase(project, 'PBXCopyFilesBuildPhase', mainTargetUuid, EMBED_PHASE)) {
      project.addBuildPhase([], 'PBXCopyFilesBuildPhase', EMBED_PHASE, mainTargetUuid, 'app_extension', '""');
    }
    const embed = buildPhase(project, 'PBXCopyFilesBuildPhase', mainTargetUuid, EMBED_PHASE);
    if (!embed.files.some((f) => f.value === product.uuid)) {
      embed.files.push({ value: product.uuid, comment: `${product.basename} in ${product.group}` });
    }
    if (!project.pbxBuildFileSection()[product.uuid]) project.addToPbxBuildFileSection(product);

    // Project navigator group.
    if (!project.pbxGroupByName(TARGET)) {
      const group = project.addPbxGroup([...sources, `${TARGET}.entitlements`], TARGET, TARGET);
      const groups = objects.PBXGroup;
      for (const key of Object.keys(groups)) {
        if (typeof groups[key] === 'object' && groups[key].name === undefined && groups[key].path === undefined) {
          project.addToPbxGroup(group.uuid, key);
        }
      }
    }
    return config;
  });

module.exports = function withCallerId(config, props) {
  const opts = resolve(config, props);
  config = withEasExtension(config, opts);
  config = withInfoPlist(config, (config) => {
    config.modResults.OlexCallerIdAppGroup = opts.groupIdentifier;
    config.modResults.OlexCallerIdExtensionId = opts.bundleIdentifier;
    return config;
  });
  config = withEntitlementsPlist(config, (config) => {
    const groups = config.modResults[GROUPS_KEY] ?? [];
    if (!groups.includes(opts.groupIdentifier)) config.modResults[GROUPS_KEY] = [...groups, opts.groupIdentifier];
    return config;
  });
  config = withTargetFiles(config, opts);
  return withTarget(config, opts);
};
