{
  description = "Builds seedetcher disk image for Raspberry Pi";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs";
    nixpkgs-unstable.url = "github:NixOS/nixpkgs/master";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, nixpkgs-unstable, utils }:
    utils.lib.eachDefaultSystem (system:
      let
        # Explicitly define host and cross packages
        hostPkgs = import nixpkgs { system = system; };  # Host system (e.g., x86_64-linux)
        crossPkgs = import nixpkgs {
          system = system;
          crossSystem = {
            config = "armv6l-unknown-linux-musleabihf";
            gcc = {
              arch = "armv6k";
              fpu = "vfp";
            };
          };
          overlays = [
            (final: prev: {
              kbd = prev.kbd.overrideAttrs (old: {
                configureFlags = (old.configureFlags or []) ++ [
                  "ac_cv_func_malloc_0_nonnull=yes"
                  "ac_cv_func_realloc_0_nonnull=yes"
                  "gl_cv_func_malloc_0_nonnull=yes"
                  "gl_cv_func_realloc_0_nonnull=yes"
                ];
              });
            })
          ];
        };
        
        crosspkgs-unstable = import nixpkgs-unstable {
          system = system;  # Changed from inherit for clarity
          crossSystem = {
            config = "armv6l-unknown-linux-musleabihf";
            gcc = {
              arch = "armv6k";
              fpu = "vfp";
            };
          };
        };

        # Inherit lib from nixpkgs
        lib = nixpkgs.lib;

        timestamp = "2009/01/03T12:15:05";
        loader-lib = "ld-musl-armhf.so.1";
      in
      {
        formatter = hostPkgs.nixpkgs-fmt;
        lib = {
          mkkernel =
            let
              pkgs = crossPkgs;
              panel-firmware = self.lib.${system}.panel-firmware;
            in
            debug: pkgs.stdenv.mkDerivation {
              name = "Raspberry Pi Linux kernel";

              src = pkgs.fetchFromGitHub {
                owner = "raspberrypi";
                repo = "linux";
              
                #rev = "dc6771425e9604650d1d57f7c69948be405f59a5";
                #hash = "sha256-ymPv/MLqDk9vRyrzEBCis9J5nZyPTti811BgnLoCnTM=";
                
                #old
                rev = "3bb5880ab3dd31f75c07c3c33bf29c5d469b28f3";
                hash = "sha256-v4ennISbEk0ApnfDRZKCJOHfO8qLdlBNlGjffkOy7LY=";

                # Remove files that introduce case sensitivity clashes on darwin.
                postFetch = ''
                  rm $out/include/uapi/linux/netfilter/xt_*.h
                  rm $out/include/uapi/linux/netfilter_ipv4/ipt_*.h
                  rm $out/include/uapi/linux/netfilter_ipv6/ip6t_*.h
                  rm $out/net/netfilter/xt_*.c
                  rm $out/tools/memory-model/litmus-tests/Z6.0+poonce*
                '';
              };

              # For reproducible builds.
              KBUILD_BUILD_TIMESTAMP = timestamp;
              KBUILD_BUILD_USER = "seedetcher";
              KBUILD_BUILD_HOST = "seedetcher.com";

              enableParallelBuilding = true;

              makeFlags = [
                "ARCH=arm"
                "CROSS_COMPILE=${pkgs.stdenv.cc.targetPrefix}"
              ];

              depsBuildBuild = [ pkgs.buildPackages.stdenv.cc ];

              nativeBuildInputs = with pkgs.buildPackages; [
                elf-header
                bison
                flex
                openssl
                bc
                perl
                util-linux
                bash
                busybox
              ];
                
              patches = [
                ./patches/kernel_missing_includes.patch
              ];

              hardeningDisable = [ "bindnow" "format" "fortify" "stackprotector" "pic" "pie" ];

              postPatch = ''
                patchShebangs scripts/config
              '';

              configurePhase = ''
                export HOSTCC=$CC_FOR_BUILD
                export HOSTCXX=$CXX_FOR_BUILD
                export HOSTAR=$AR_FOR_BUILD
                export HOSTLD=$LD_FOR_BUILD

                make $makeFlags -j$NIX_BUILD_CORES \
                  HOSTCC=$HOSTCC HOSTCXX=$HOSTCXX HOSTAR=$HOSTAR HOSTLD=$HOSTLD \
                  CC=$CC OBJCOPY=$OBJCOPY OBJDUMP=$OBJDUMP READELF=$READELF \
                  HOSTCFLAGS="-D_POSIX_C_SOURCE=200809L" \
                  bcmrpi_defconfig

                ./scripts/config --set-str EXTRA_FIRMWARE panel.bin
                ./scripts/config --set-str EXTRA_FIRMWARE_DIR ${panel-firmware}
                # Disable networking (including bluetooth).
                ./scripts/config --disable NET
                ./scripts/config --disable INET
                ./scripts/config --disable NETFILTER
                ./scripts/config --disable PROC_SYSCTL
                ./scripts/config --disable FSCACHE
                # There's no need for security models, and leaving it enabled
                # leads to build errors because of the files removed in postFetch above.
                ./scripts/config --disable SECURITY
                # Disable sound support.
                ./scripts/config --disable SOUND
                # Disable features we don't need.
                ./scripts/config --disable EXT4_FS
                ./scripts/config --disable F2FS_FS
                ./scripts/config --disable PSTORE
                ./scripts/config --disable INPUT_TOUCHSCREEN
                ./scripts/config --disable RC_MAP
                ./scripts/config --disable NAMESPACES
                ./scripts/config --disable INPUT
                # Enable v4l2
                ./scripts/config --enable MEDIA_SUPPORT
                ./scripts/config --enable VIDEO_V4L2
                ./scripts/config --enable VIDEO_DEV
                # Enable camera driver.
                ./scripts/config --enable I2C_BCM2835
                ./scripts/config --enable I2C_MUX
                ./scripts/config --enable REGULATOR_FIXED_VOLTAGE
                ./scripts/config --enable I2C_MUX_PINCTRL
                ./scripts/config --enable VIDEO_BCM2835_UNICAM
                ./scripts/config --enable VIDEO_CODEC_BCM2835
                ./scripts/config --enable VIDEO_ISP_BCM2835
                # Raspberry camera module 1.
                ./scripts/config --enable VIDEO_OV5647
                # Raspberry camera module 3.
                ./scripts/config --enable VIDEO_IMX708
                ./scripts/config --enable VIDEO_DW9807_VCM
                # Enable SPI.
                ./scripts/config --enable SPI_BCM2835
                # Enable FTDI USB serial driver.
                ./scripts/config --enable USB_SERIAL
                ./scripts/config --enable USB_SERIAL_FTDI_SIO
                # Enable USB printer support
                ./scripts/config --enable USB_PRINTER
                # Disable HDMI framebuffer device.
                ./scripts/config --disable FB_BCM2708
                # Enable display driver.
                ./scripts/config --enable BACKLIGHT_GPIO
                ./scripts/config --enable DRM
                ./scripts/config --enable DRM_PANEL_MIPI_DBI
                ./scripts/config --disable LOGO
                ./scripts/config --enable FRAMEBUFFER_CONSOLE_DEFERRED_TAKEOVER
                # Enable slower but better supported USB driver.
                ./scripts/config --disable USB_DWCOTG
                ./scripts/config --enable USB_DWC2

                # For Raspberry Pi Zero 2.
                ./scripts/config --enable ARCH_MULTI_V7
                ./scripts/config --enable ARM_ERRATA_643719
                # Enabling VDSO for some reason introduces enough differences between
                # Linux and macOS that the resulting kernel image differs.
                ./scripts/config --disable VDSO
              '' + (if debug then ''
                ./scripts/config --enable SERIAL_8250
                ./scripts/config --enable SERIAL_8250_CONSOLE
                ./scripts/config --enable TTY
                ./scripts/config --enable VT
                ./scripts/config --enable VT_CONSOLE
                ./scripts/config --enable USB_G_SERIAL
                ./scripts/config --enable VFP
              '' else "");

              buildPhase = ''
                export HOSTCC=$CC_FOR_BUILD
                export HOSTCXX=$CXX_FOR_BUILD
                export HOSTAR=$AR_FOR_BUILD
                export HOSTLD=$LD_FOR_BUILD

                make $makeFlags -j$NIX_BUILD_CORES \
                  HOSTCC=$HOSTCC HOSTCXX=$HOSTCXX HOSTAR=$HOSTAR HOSTLD=$HOSTLD \
                  CC=$CC OBJCOPY=$OBJCOPY OBJDUMP=$OBJDUMP READELF=$READELF \
                  HOSTCFLAGS="-D_POSIX_C_SOURCE=200809L" \
                  zImage dtbs
              '';

              installPhase = ''
                mkdir -p $out/overlays
                cp arch/arm/boot/zImage $out/kernel.img
                cp arch/arm/boot/dts/*rpi-zero*.dtb $out/
                pushd arch/arm/boot/dts/overlays
                cp dwc2.dtbo ov5647.dtbo imx708.dtbo mipi-dbi-spi.dtbo disable-bt.dtbo disable-wifi.dtbo $out/overlays/
                popd
              '';

              allowedReferences = [ ];
            };
          panel-firmware =
            let
              pkgs = hostPkgs;
              # firmware is the commands required to initialize the st7789 panel.
              firmware = pkgs.writeText "firmware.txt" ''
                command 0x11 # exit sleep mode
                delay 120

                command 0x3A 0x05 # set pixel format 16-bit
                command 0xB2 0x05 0x05 0x00 0x33 0x33 # PORCTRL
                command 0xB7 0x75 # GCTRL
                command 0xC2 0x01 0x0FF # VDVVRHEN
                command 0xC3 0x13 # VHRS
                command 0xC4 0x20 # VDVS
                command 0xBB 0x22 # VCOMS
                command 0xC5 0x20 # VCMOFSET
                command 0xD0 0xA4 0xA1 # PWRCTRL1

                # gamma
                command 0xE0 0xD0 0x05 0x0A 0x09 0x08 0x05 0x2E 0x44 0x45 0x0F 0x17 0x16 0x2B 0x33
                command 0xE1 0xD0 0x05 0x0A 0x09 0x08 0x05 0x2E 0x43 0x45 0x0F 0x16 0x16 0x2B 0x33

                command 0x29 # display on
                command 0x21 # invert mode

                command 0x36 0x60 # set address mode
              '';
              firmware-converter = pkgs.fetchurl {
                url = "https://raw.githubusercontent.com/notro/panel-mipi-dbi/374b15f78611c619c381c643c5b3a8b5d23f479b/mipi-dbi-cmd";
                hash = "sha256-ZOx6l84IFpyooPFdgumCL2WUBqCKi0G36X6H8QjyNEc=";
              };
            in
            pkgs.stdenvNoCC.mkDerivation {
              name = "panel-firmware";

              dontUnpack = true;

              installPhase = ''
                mkdir $out
                ${pkgs.python3}/bin/python3 ${firmware-converter} $out/panel.bin ${firmware}
              '';
            };
          mkinitramfs = debug:
            let
              pkgs = hostPkgs;
              busyboxStatic = crossPkgs.pkgsStatic.busybox;
              bashStatic = crossPkgs.pkgsStatic.bash;
              straceStatic = crossPkgs.pkgsStatic.strace;
              binutilsStatic = crossPkgs.pkgsStatic.binutils; #for readelf
              mupdfHeadlessStatic = self.packages.${system}.mupdf_headless;

              fontFile = ./font/martianmono/MartianMono_Condensed-Regular.ttf;

              controller =
                if debug then
                  self.packages.${system}.controller-debug
                else
                  self.packages.${system}.controller;
                
              # Define initScript separately from buildPhase
              initScript = pkgs.writeTextFile {
                name = "init";
                text = builtins.readFile ./init.sh;
              };

            in
            pkgs.stdenvNoCC.mkDerivation {
              name = "initramfs";

              dontUnpack = true;
              buildPhase = ''
                # Create initramfs with controller program and libraries.
                mkdir initramfs
                cp ${controller}/bin/controller initramfs/controller
                cp -R "${self.packages.${system}.camera-driver}"/* initramfs/
                # Set constant mtimes and permissions for determinism.
                chmod 0755 `find initramfs`|| true
                chmod 0755 initramfs/lib/ld-musl-armhf.so.1 || true
                # Remove symlinks as their permissions are not consistent across platforms.
                # Fortunately we only need symlinks during linking to libraries, not at runtime.
                rm -f `find initramfs -type l`
                ${pkgs.coreutils}/bin/touch -d '${timestamp}' `find initramfs`

                # Add font directory and file
                mkdir -p initramfs/font/martianmono
                cp ${fontFile} initramfs/font/martianmono/MartianMono_Condensed-Regular.ttf
                chmod 0644 initramfs/font/martianmono/MartianMono_Condensed-Regular.ttf
                ${pkgs.coreutils}/bin/touch -d '${timestamp}' initramfs/font/martianmono/MartianMono_Condensed-Regular.ttf
    
                # Create essential initramfs directories
                mkdir -p initramfs/{bin,lib,share,dev,proc,sys,run,tmp} # Added tmp here

                # Set permissions for /tmp (1777: readable/writable by all, sticky bit)
                chmod 1777 initramfs/tmp
                ${pkgs.coreutils}/bin/touch -d '${timestamp}' initramfs/tmp

                # Create /dev/console as an empty file (kernel will handle it)
                touch initramfs/dev/console
                chmod 622 initramfs/dev/console
                touch initramfs/dev/null
                chmod 666 initramfs/dev/null

                # Create mount points for system services
                mkdir -p initramfs/dev/{pts,shm}

                # Ensure write permissions before copying files
                chmod -R u+w initramfs/bin initramfs/lib || true

                # Copy essential binaries
                cp -a ${busyboxStatic}/bin/* initramfs/bin/
                cp -a ${straceStatic}/bin/strace initramfs/bin/
                cp -L --no-preserve=mode ${binutilsStatic}/bin/readelf initramfs/bin/readelf
                chmod +x initramfs/bin/readelf

                # Debug output
                echo "Verifying readelf:"
                ls -alh initramfs/bin/readelf
                file initramfs/bin/readelf

                # Copy readelf as a file, ensuring it's dereferenced and not a symlink
                target=$(readlink -f "${binutilsStatic}/bin/readelf")
                cp -L --no-preserve=mode "$target" initramfs/bin/readelf || echo "Failed to copy readelf"
                chmod +x initramfs/bin/readelf 2>/dev/null || echo "Warning: Could not change permissions of readelf"

                cp -a ${mupdfHeadlessStatic}/bin/mutool initramfs/bin/ || echo "Failed to copy mutool from ${mupdfHeadlessStatic}/bin/"
                chmod +x initramfs/bin/mutool || echo "chmod failed, listing dir: $(ls -alh initramfs/bin/)"


                # Only create symlinks if they do not already exist
                for cmd in ls cat echo sh rm mkdir rmdir cp mv touch; do
                  if [ ! -e initramfs/bin/$cmd ]; then
                    ln -s busybox initramfs/bin/$cmd
                  fi
                done

                # Set proper permissions
                chmod -R 0755 initramfs/bin initramfs/lib || true

                cp ${initScript} initramfs/init
                # Make init executable
                chmod +x initramfs/init
            
                ${pkgs.findutils}/bin/find initramfs -mindepth 1 -printf '%P\n'\
                  | sort \
                  | ${pkgs.cpio}/bin/cpio -D initramfs --reproducible -H newc -o --owner +0:+0 --quiet \
                  | ${pkgs.gzip}/bin/gzip > initramfs.cpio.gz
              '';

              installPhase = ''
                mkdir -p $out
                # cmyk fix permissions
                chmod 0755 initramfs/lib/ld-musl-armhf.so.1 || true
                cp initramfs.cpio.gz $out/

                ls -alh initramfs/  # Verify initramfs contents before copying
                ls -alh initramfs/bin/  # Check if binaries are actually copied
                ls -alh initramfs/lib/
              '';

              allowedReferences = [ ];
            };
          mkimage = { debug, usbMode ? "gadget" }:
            let
              pkgs = hostPkgs;
              firmware = self.packages.${system}.firmware;
              kernel =
                if debug then
                  self.packages.${system}.kernel-debug
                else
                  self.packages.${system}.kernel;

              initramfs = self.lib.${system}.mkinitramfs debug;
              img-name =
                let
                  base = if usbMode == "host" then "seedetcher-host" else "seedetcher";
                in
                if debug then "${base}-debug.img" else "${base}.img";

              cmdlinetxt =
                let
                  cmdlineBase = "console=tty1 rootwait";
                  cmdlineSerial = "console=serial0,115200 " + cmdlineBase;
                in
                if usbMode == "gadget" then
                  pkgs.writeText "cmdline.txt" "console=ttyGS0,115200 ${cmdlineBase} modules-load=dwc2,g_serial g_serial.n_ports=2"
                else if debug then
                  pkgs.writeText "cmdline.txt" "${cmdlineSerial} modules-load=dwc2,usblp"
                else
                  pkgs.writeText "cmdline.txt" "${cmdlineBase} modules-load=dwc2,usblp";

              configtxt =
                let
                  usbOverlay = if usbMode == "gadget" then "dtoverlay=dwc2" else "dtoverlay=dwc2,dr_mode=host";
                in
                pkgs.writeText "config.txt" ''
                  initramfs initramfs.cpio.gz followkernel
                  disable_splash=1
                  boot_delay=0
                  force_turbo=1
                  camera_auto_detect=1
                  dtoverlay=mipi-dbi-spi
                  dtparam=width=240
                  dtparam=height=240
                  dtparam=width-mm=23
                  dtparam=height-mm=23
                  dtparam=reset-gpio=27
                  dtparam=dc-gpio=25
                  dtparam=backlight-gpio=24
                  dtparam=write-only
                  dtparam=speed=40000000
                  ${usbOverlay}
                  dtoverlay=disable-bt
                  dtoverlay=disable-wifi
                '';
            in
            pkgs.stdenvNoCC.mkDerivation {
              name = "disk-image";

              dontUnpack = true;
              buildPhase = ''
                sectorsToBlocks() {
                  echo $(( ( "$1" * 512 ) / 1024 ))
                }

                sectorsToBytes() {
                  echo $(( "$1" * 512 ))
                }

                # Create disk image.
                dd if=/dev/zero of=disk.img bs=1M count=64
                ${pkgs.util-linux}/bin/sfdisk disk.img <<EOF
                  label: dos
                  label-id: 0xceedb0ad

                  disk.img1 : type=c, bootable
                EOF

                # Create boot partition.
                START=$(${pkgs.util-linux}/bin/fdisk -l -o Start disk.img|tail -n 1)
                SECTORS=$(${pkgs.util-linux}/bin/fdisk -l -o Sectors disk.img|tail -n 1)
                ${pkgs.dosfstools}/bin/mkfs.vfat --invariant -i deadbeef -n seedetcher disk.img --offset $START $(sectorsToBlocks $SECTORS)
                OFFSET=$(sectorsToBytes $START)

                # Copy boot files.
                mkdir -p boot/overlays overlays
                cp ${cmdlinetxt} boot/cmdline.txt
                cp ${configtxt} boot/config.txt
                cp ${firmware}/boot/bootcode.bin ${firmware}/boot/start.elf \
                  ${firmware}/boot/fixup.dat \
                  ${kernel}/kernel.img ${kernel}/*.dtb \
                  ${initramfs}/initramfs.cpio.gz boot/
                cp ${kernel}/overlays/* overlays/

                chmod 0755 `find boot overlays`
                ${pkgs.coreutils}/bin/touch -d '${timestamp}' `find boot overlays`
                ${pkgs.mtools}/bin/mcopy -bpm -i "disk.img@@$OFFSET" boot/* ::
                # mcopy doesn't copy directories deterministically, so rely on sorted shell globbing
                # instead.
                ${pkgs.mtools}/bin/mcopy -bpm -i "disk.img@@$OFFSET" overlays/* ::overlays
              '';

              installPhase = ''
                mkdir -p $out
                cp disk.img $out/${img-name}
              '';

              allowedReferences = [ ];
            };
          mkcontroller = debug:
            let
              libcamera = self.packages.${system}.libcamera;
              pkgs = crossPkgs;
              pkgs-unstable = crosspkgs-unstable;
              tags = builtins.concatStringsSep "," ([ "netgo" ] ++ (if debug then [ "debug" ] else [ ]));
            in
            pkgs.stdenv.mkDerivation {
              name = "controller";
              src = ./.;

              nativeBuildInputs = with pkgs.buildPackages; [
                pkgs-unstable.buildPackages.go_1_23
                nukeReferences
              ];

              CGO_CXXFLAGS="-I${libcamera}/include";
              CGO_LDFLAGS="-L${libcamera}/lib -static-libstdc++ -static-libgcc";
              CGO_ENABLED="1";
              GOOS="linux";
              GOARCH="arm";
              GOARM="6";
              # -buildmode=pie is required by musl.
              GOFLAGS="-buildmode=pie -tags=${tags} -gccgoflags=-march=armv6";

              buildPhase = ''
                HOME="$PWD/gohome" \
                GOMODCACHE=${self.packages.${system}.go-deps} \
                  go build -trimpath -ldflags="-s -w" ./cmd/controller
              '';

              installPhase = ''
                mkdir -p $out/bin
                cp controller $out/bin
              '';

              fixupPhase = ''
                patchelf --set-rpath "/lib" \
                  --set-interpreter "/lib/${loader-lib}" \
                  $out/bin/controller
                nuke-refs $out/bin/controller
                prefix=${pkgs.stdenv.targetPlatform.config}
                # Strip the indeterministic Go build id.
                "$prefix"-objcopy -R .note.go.buildid $out/bin/controller
              '';

              allowedReferences = [ ];
            };
        };
        packages =
          {
            go-deps = let pkgs = hostPkgs; pkgs-unstable = crosspkgs-unstable; in pkgs.stdenvNoCC.mkDerivation {
              pname = "go-deps";
              version = "1";

              src = builtins.filterSource
                (path: type: baseNameOf path == "go.mod" || baseNameOf path == "go.sum")
                ./.;

              dontBuild = true;

              nativeBuildInputs = with pkgs.buildPackages; [
                cacert
                pkgs-unstable.buildPackages.go_1_23
              ];

              installPhase = ''
                mkdir -p $out
                export HOME="$PWD/gohome"
                GOMODCACHE=$out go mod download
              '';

              outputHashMode = "recursive";
              outputHashAlgo = "sha256";
              outputHash = "Hz5M3tgnfa5xdGyop9TDo3m/YIrDs0M9wNBIJMbewBA=";
            };
            controller = self.lib.${system}.mkcontroller false;
            controller-debug = self.lib.${system}.mkcontroller true;
            libcamera =
              let
                pkgs = crossPkgs;
                cpu = pkgs.targetPlatform.parsed.cpu;
                cross-file = pkgs.writeText "cross-file.conf" ''
                  [properties]
                  needs_exe_wrapper = true

                  [host_machine]
                  system = 'linux'
                  cpu_family = '${cpu.family}'
                  cpu = '${cpu.name}'
                  endian = 'little'
                '';
              in
              pkgs.stdenv.mkDerivation rec {
                pname = "libcamera";
                version = "0.3";
                patch = "0";

                src = pkgs.fetchgit {
                  url = "https://git.libcamera.org/libcamera/libcamera.git";
                  rev = "v${version}.${patch}";
                  hash = "sha256-eCtOtdjpwn0S56ZyRVdG1QCBk1KGPh8YTXD50xev7Bc=";
                };

                patches = [
                  # Disable IPA signature validation: we don't need it and it depends
                  # on openssl which is fiddly to build reproducibly.
                  ./patches/libcamera_disable_signature.patch
                ];

                postPatch = ''
                  patchShebangs utils/
                '';

                dontUseNinjaInstall = true;
                dontUseMesonConfigure = true;

                nativeBuildInputs = with pkgs.pkgsBuildHost; [
                  meson
                  ninja
                  pkg-config
                  python3
                  python3Packages.jinja2
                  python3Packages.pyyaml
                  python3Packages.ply
                ];

                buildInputs = with pkgs.pkgsStatic; [
                  libyaml
                ];

                configurePhase = ''
                  LDFLAGS="-static-libstdc++ -static-libgcc" meson setup build \
                    -Dv4l2=false \
                    -Dqcam=disabled \
                    -Dcam=disabled \
                    -Dgstreamer=disabled \
                    -Dtracing=disabled \
                    -Dpipelines=rpi/vc4 \
                    -Ddocumentation=disabled \
                    -Dlc-compliance=disabled \
                    --cross-file=${cross-file} \
                    --buildtype=plain \
                    --prefix=/ \
                    --libdir=lib
                  cd build
                '';

                installPhase = ''
                  mkdir -p $out/lib/libcamera $out/share/libcamera/ipa/rpi/vc4 $out/include
                  cp ./src/libcamera/libcamera.so.${version}.${patch} $out/lib/libcamera.so.${version}
                  cp ./src/libcamera/base/libcamera-base.so.${version}.${patch} $out/lib/libcamera-base.so.${version}
                  ln -s $out/lib/libcamera.so.${version} $out/lib/libcamera.so
                  ln -s $out/lib/libcamera-base.so.${version} $out/lib/libcamera-base.so
                  cp ./src/ipa/rpi/vc4/ipa_rpi_vc4.so $out/lib/libcamera
                  cp ../src/ipa/rpi/vc4/data/*.json $out/share/libcamera/ipa/rpi/vc4/
                  cp -a ../include/libcamera include/libcamera $out/include
                  patchelf --set-rpath "/lib" `find $out/lib -type f`
                '';

                strictDeps = true;
                enableParallelBuilding = true;

                allowedReferences = [ ];
              };
            camera-driver = let pkgs = crossPkgs; in pkgs.stdenv.mkDerivation {
              name = "camera-driver";

              dontUnpack = true;

              nativeBuildInputs = with pkgs.buildPackages; [
                binutils
              ];

              installPhase = ''
                mkdir -p $out/lib/libcamera
                LIBCAM="${self.packages.${system}.libcamera}"
                prefix=${pkgs.stdenv.targetPlatform.config}

                # Copy the dynamic link while stripping it of indeterministic sections.
                "$prefix"-objcopy -R .note.gnu.build-id "${pkgs.stdenv.cc.libc}/lib/${loader-lib}" \
                  $out/lib/${loader-lib}
                cp -R "$LIBCAM/lib/" $out/
                cp -R "$LIBCAM/share/" $out/
              '';

              strictDeps = true;

              allowedReferences = [ ];
            };
            kernel = self.lib.${system}.mkkernel false;
            kernel-debug = self.lib.${system}.mkkernel true;
            firmware = hostPkgs.fetchFromGitHub {
              owner = "raspberrypi";
              repo = "firmware";
              rev = "9de4cecc88873d154455b2c254a2bfbb2be8c1b7";
              sparseCheckout = [ "boot" ];
              hash = "sha256-rSZ3sUnSmBcsIqc+K91GDs5qlqiP+j9zf9gM2lqzr8w=";
            };
            initramfs = self.lib.${system}.mkinitramfs false;
            initramfs-debug = self.lib.${system}.mkinitramfs true;
            image = self.lib.${system}.mkimage { debug = false; usbMode = "gadget"; };
            image-debug = self.lib.${system}.mkimage { debug = true; usbMode = "gadget"; };
            image-host = self.lib.${system}.mkimage { debug = false; usbMode = "host"; };
            image-host-debug = self.lib.${system}.mkimage { debug = true; usbMode = "host"; };
            # reload the controller binary to a running seedetcher debug image.
            reload = let pkgs = hostPkgs; in pkgs.writeShellScriptBin "reload" ''
              #!/bin/sh
              set -x  # Enable debug mode
              
              # Set default USB device if not provided
              if [ -z "$1" ]; then
                  USBDEV="/dev/ttyACM1"
              else
                  USBDEV="$1"
              fi
              
              if [ ! -e "$USBDEV" ]; then
                  echo "error: USB device $USBDEV not found"
                  exit 1
              fi
              
              PROG="${self.packages.${system}.controller-debug}/bin/controller"

              # Ensure USB serial is in raw mode before reloading
              stty -F $USBDEV1 raw -echo
              echo "" > $USBDEV1

              echo "reload $(wc -c < "$PROG")" > "$USBDEV"
              cat "$PROG" > "$USBDEV"
              exec cat "$USBDEV"
            '' // {
              buildInputs = with pkgs; [
                bash  # Explicitly include Bash
                (glibcLocales.override { allLocales = false; locales = [ "C.utf8" ]; })
              ];
            };
            # reload-fast is a faster, but impure, way of reloading the controller binary
            # from a developer shell.
            reload-fast = let pkgs = hostPkgs; in pkgs.writeShellScriptBin "reload-fast" ''
              set -e
              USBDEV=$1
              if [ -z "$1" ]; then
                  echo "error: specify USB device"
                  exit 1
              fi

              eval "$buildPhase"
              eval "$installPhase"
              eval "$fixupPhase"

              PROG=outputs/out/bin/controller
              echo "reload $(wc -c < "$PROG")" > "$USBDEV"
              cat "$PROG" > "$USBDEV"
              exec cat "$USBDEV"
            '';
            mkRelease = let pkgs = hostPkgs; in pkgs.writeShellScriptBin "mk-release" ''
              set -eu

              VERSION=$1
              if [ -z "$VERSION" ]; then
                  echo "error: specify version"
                  exit 1
              fi

              flake="github:seedetcher/seedetcher/$VERSION"
              nix build "$flake"
              nix run "$flake"#stamp-release $VERSION

              if [[ -v SSH_SIGNING_KEY ]]; then
                ssh-keygen -Y sign -f "$SSH_SIGNING_KEY" -n seedetcher.img seedetcher-"$VERSION".img
              fi
            '';
            stamp-release = let pkgs = hostPkgs; in pkgs.writeShellScriptBin "stamp-release" ''
              set -eu

              # For determinism.
              export TZ=UTC
              VERSION=$1
              if [ -z "$VERSION" ]; then
                  echo "error: specify version"
                  exit 1
              fi
              TMPDIR="$(mktemp -d)"
              trap 'rm -rf -- "$TMPDIR"' EXIT

              src="result/seedetcher.img"
              dst="seedetcher-$VERSION.img"

              # Append the version string to the kernel cmdline, to be read by the controller binary.
              START=$(${pkgs.util-linux}/bin/fdisk -l -o Start $src|tail -n 1)
              OFFSET=$(( $START*512 ))
              ${pkgs.mtools}/bin/mcopy -bpm -i "$src@@$OFFSET" ::cmdline.txt "$TMPDIR/"
              echo -n " sh_version=$VERSION" >> "$TMPDIR/cmdline.txt"
              # preserve attributes for determinism.
              chmod 0755 "$TMPDIR/cmdline.txt"
              ${pkgs.coreutils}/bin/touch -d '${timestamp}' "$TMPDIR/cmdline.txt"
              cp "$src" "$dst"
              chmod +w "$dst"
              ${pkgs.mtools}/bin/mdel -i "$dst@@$OFFSET" ::cmdline.txt
              ${pkgs.mtools}/bin/mcopy -bpm -i "$dst@@$OFFSET" "$TMPDIR/cmdline.txt" ::
            '';

          mupdf_headless = crossPkgs.stdenv.mkDerivation {
            name = "mupdf-headless";
            version = "1.25.4";

            src = crossPkgs.fetchurl {
              url = "https://mupdf.com/downloads/archive/mupdf-1.25.4-source.tar.gz";
              # Replace with the correct hash from your build logs or nix-prefetch-url
              hash = "sha256-dLlDA4/oFZS/f8ViHGC8pYiyhH8NRvsumWUqIfoNlJE="; # Update this!
            };

            nativeBuildInputs = with crossPkgs.buildPackages; [
              pkg-config
            ];
            buildInputs = with crossPkgs.pkgsStatic; [
              zlib
              freetype
              libpng
              musl # Explicitly include musl for static linking
            ];

            # Ensure static linking by passing -static and adjusting LDFLAGS
            buildPhase = ''
              make HAVE_X11=no HAVE_GLUT=no HAVE_GLFW=no prefix=/usr \
                CC="${crossPkgs.stdenv.cc.targetPrefix}cc -static" \
                AR=${crossPkgs.stdenv.cc.targetPrefix}ar \
                LDFLAGS="-static -L${crossPkgs.pkgsStatic.zlib}/lib -L${crossPkgs.pkgsStatic.freetype}/lib -L${crossPkgs.pkgsStatic.libpng}/lib -L${crossPkgs.pkgsStatic.musl}/lib" \
                libs # Build libmupdf first
              make HAVE_X11=no HAVE_GLUT=no HAVE_GLFW=no prefix=/usr \
                CC="${crossPkgs.stdenv.cc.targetPrefix}cc -static" \
                AR=${crossPkgs.stdenv.cc.targetPrefix}ar \
                LDFLAGS="-static -L${crossPkgs.pkgsStatic.zlib}/lib -L${crossPkgs.pkgsStatic.freetype}/lib -L${crossPkgs.pkgsStatic.libpng}/lib -L${crossPkgs.pkgsStatic.musl}/lib" \
                apps # Build mutool statically
            '';

            installPhase = ''
              mkdir -p $out/bin $out/lib
              cp build/release/libmupdf.a $out/lib/
              cp build/release/mutool $out/bin/
              ${crossPkgs.stdenv.cc.targetPrefix}strip $out/bin/mutool
            '';

            hardeningDisable = ["all"];
            dontStrip = false;
            allowedReferences = []; # Keep this for now, should be empty if fully static
          };

            default = self.packages.${system}.image;
          };
          # developer shell for running .#reload-fast.
          devShells.default = hostPkgs.mkShell {
            packages = [ self.packages.${system}.controller-debug ];
            shellHook = ''
              echo "🚀 Welcome to the SeedEtcher dev shell!"
              export PS1='\[\e[1;32m\][seedetcher-dev]\[\e[0m\] \w \$ '
              export SHELL=${hostPkgs.bash}/bin/bash
              export CC=arm-linux-gnueabihf-gcc
              export CXX=arm-linux-gnueabihf-g++
              export GOARCH=arm
              export GOARM=6
              export GOOS=linux
              export CGO_ENABLED=1
              export CGO_CXXFLAGS="-I${self.packages.${system}.libcamera}/include"
              export CGO_LDFLAGS="-L${self.packages.${system}.libcamera}/lib"
            '';
          };
      });
}
