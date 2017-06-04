.. _installation:

Installation
------------

Overview
========

Download source from `GitHub <https://github.com/oracle/odpi>`__.

A sample Makefile is provided if you wish to build ODPI-C as a shared library.
Otherwise, include the ODPI-C source code in your project. On Windows, Visual
Studio 2008 or higher is required. On macOS, Xcode 6 or higher is required. On
Linux, GCC 4.4 or higher is required.

ODPI-C requires Oracle Client libraries to be installed.  By far the simplest
Oracle Client that can be installed is the free `Oracle Instant Client
<http://www.oracle.com/technetwork/database/features/instant-client/index.html>`__.
Only the "Basic" or "Basic Light" package is required. Oracle Client libraries
are also available in any Oracle Database installation or full Oracle Client
installation.

ODPI-C uses the shared library loading mechanism available on each supported
platform to load the Oracle Client library at runtime. This allows code using
ODPI-C to be built only once, and then run using Oracle Client 11.2, 12.1 or
12.2 libraries.  If an Oracle Client cannot be found at runtime, the error
"DPI-1047: Oracle Client library cannot be loaded" is raised. The following
sections explain how to ensure the Oracle Client is installed and configured
correctly on the various platforms so that ODPI-C is able to find it.

ODPI-C has been tested on Linux, Windows and macOS.  Other platforms should
also work but have not been tested.  On Windows, Visual Studio 2008 or higher
is required.  On macOS, Xcode 6 or higher is required.  On Linux, GCC 4.4 or
higher is required.


Oracle Client and Database Versions
===================================

ODPI-C can use Oracle Client 11.2, 12.1 or 12.2 libraries.

Oracle Client libraries allow connection to older and newer databases.
In summary, Oracle Client 12.2 can connect to Oracle Database 11.2 or
greater. Oracle Client 12.1 can connect to Oracle Database 10.2 or
greater. Oracle Client 11.2 can connect to Oracle Database 9.2 or
greater.  For additional information on which Oracle Database releases
are supported by which Oracle client versions, please see `Doc ID
207303.1 <https://support.oracle.com/epmos/faces/DocumentDisplay?id=207303.1>`__.

Since a single ODPI-C binary can use multiple client versions and
access multiple database versions, it is important your application is
tested in your intended release environments.  Newer Oracle clients
support new features, such as the `oraaccess.xml
<https://docs.oracle.com/database/122/LNOCI/more-oci-advanced-topics.htm#LNOCI73052>`__
external configuration file available with 12.1 or later clients, and
`session pool enhancements
<http://docs.oracle.com/database/122/LNOCI/release-changes.htm#LNOCI005>`__
to dead connection detection in 12.2 clients.

The function :func:`dpiContext_getClientVersion()` can be used to determine
which Oracle Client version is in use and the function
:func:`dpiConn_getServerVersion()` can be used to determine which Oracle
Database version a connection is accessing. These can then be used to adjust
application behavior accordingly. Attempts to use some Oracle features that are
not supported by a particular client/server combination may result in runtime
errors. These include:

    - when attempting to access attributes that are not supported by the
      current Oracle Client library you will get the error "ORA-24315: illegal
      attribute type"

    - when attempting to use implicit results with Oracle Client 11.2
      against Oracle Database 12c you will get the error "ORA-29481:
      Implicit results cannot be returned to client"

    - when attempting to get array DML row counts with Oracle Client
      11.2 you will get the error "DPI-1013: not supported"


Linux
=====

On Linux, ODPI-C first searches for a library called "libclntsh.so" using the
`standard library search order
<http://man7.org/linux/man-pages/man8/ld.so.8.html>`__. If this is not found,
it will then search for "libclntsh.so.12.1" and then for "libclntsh.so.11.1"
before returning an error.


Instant Client RPM
++++++++++++++++++

To run ODPI-C applications with Instant Client RPMs:

1. Download the "Basic" or "Basic Light" RPM from here (`64-bit
   <http://www.oracle.com/technetwork/topics/linuxx86-64soft-092277.html>`__
   or `32-bit
   <http://www.oracle.com/technetwork/topics/linuxsoft-082809.html>`__).

2. Install the downloaded RPM with sudo or as the root user. For example::

       sudo yum install oracle-instantclient12.2-basic-12.2.0.1.0-1.x86_64.rpm

3. Set the environment variable ``LD_LIBRARY_PATH`` to the appropriate
   directory for the Instant Client version. For example::

       export LD_LIBRARY_PATH=/usr/lib/oracle/12.2/client64/lib:$LD_LIBRARY_PATH

   Alternatively, if there is no other Oracle software on the machine that will
   be impacted, permanently add Instant Client to the runtime link path. For
   example, with sudo or as the root user::

       sudo sh -c "echo /usr/lib/oracle/12.2/client64/lib > /etc/ld.so.conf.d/oracle-instantclient.conf"
       sudo ldconfig


Instant Client Zip
++++++++++++++++++

To run ODPI-C applications with Instant Client zip files:

1. Download the "Basic" or "Basic Light" zip file from here (`64-bit
   <http://www.oracle.com/technetwork/topics/linuxx86-64soft-092277.html>`__
   or `32-bit
   <http://www.oracle.com/technetwork/topics/linuxsoft-082809.html>`__).

2. Unzip the package into a single directory that is accessible to your
   application. For example::

       mkdir -p /opt/oracle
       cd /opt/oracle
       unzip instantclient-basic-linux.x64-12.2.0.1.0.zip

3. Install the ``libaio`` package with sudo or as the root user. For example::

       sudo yum install libaio

   On some Linux distributions this package is called ``libaio1`` instead.

4. Set the environment variable ``LD_LIBRARY_PATH`` to the appropriate
   directory for the Instant Client version. For example::

       export LD_LIBRARY_PATH=/opt/oracle/instantclient_12_2:$LD_LIBRARY_PATH

   Alternatively, if there is no other Oracle software on the machine that will
   be impacted, permanently add Instant Client to the runtime link path. For
   example, with sudo or as the root user::

       sudo sh -c "echo /opt/oracle/instantclient_12_2 > /etc/ld.so.conf.d/oracle-instantclient.conf"
       sudo ldconfig


Local Database
++++++++++++++

To run ODPI-C applications with a local Oracle Database installation:

1. Set required Oracle environment variables by running the Oracle environment
   script. For example::

       source /usr/local/bin/oraenv

   For Oracle XE, run::

       source /u01/app/oracle/product/11.2.0/xe/bin/oracle_env.sh

2. Set the environment variable ``LD_LIBRARY_PATH`` if it has not already been
   set. For example::

       export LD_LIBRARY_PATH=$ORACLE_HOME/lib:$LD_LIBRARY_PATH


Windows
=======

On Windows, ODPI-C searches for a library called "OCI.dll" using the
`standard library search order
<https://msdn.microsoft.com/en-us/library/windows/desktop/ms682586(v=vs.85).aspx>`__.

Oracle Client libraries require the presence of the correct Visual Studio
redistributable.

    - 11.2 : `VS 2005 64-bit <https://www.microsoft.com/en-us/download/details.aspx?id=18471>`__ or `VS 2005 32-bit <https://www.microsoft.com/en-ca/download/details.aspx?id=3387>`__
    - 12.1 : `VS 2010 <https://support.microsoft.com/en-us/kb/2977003#bookmark-vs2010>`__
    - 12.2 : `VS 2013 <https://support.microsoft.com/en-us/kb/2977003#bookmark-vs2013>`__


Instant Client Zip
++++++++++++++++++

To run ODPI-C applications with Instant Client zip files:

1. Download the "Basic" or "Basic Light" zip file from here (`64-bit
   <http://www.oracle.com/technetwork/topics/winx64soft-089540.html>`__ or
   `32-bit <http://www.oracle.com/technetwork/topics/winsoft-085727.html>`__).

2. Unzip the package into a single directory that is accessible to your
   application.

3. Set the environment variable ``PATH`` to include the path that you
   created in step 2. For example, on Windows 7, update ``PATH`` in
   Control Panel -> System -> Advanced System Settings -> Advanced ->
   Environment Variables -> System Variables -> PATH.


Local Database
++++++++++++++

To run ODPI-C applications with a local Oracle Database installation:

1. Set the environment variable ``PATH`` to include the path that contains
   OCI.dll, if it is not already set. For example, on Windows 7, update
   ``PATH`` in Control Panel -> System -> Advanced System Settings ->
   Advanced -> Environment Variables -> System Variables -> PATH.


macOS
=====

On macOS, ODPI-C first searches for a library called "libclntsh.dylib" using
the `standard library search order
<https://developer.apple.com/library/content/documentation/DeveloperTools/Conceptual/DynamicLibraries/100-Articles/DynamicLibraryUsageGuidelines.html>`__. If
this is not found, it will then search for "libclntsh.dylib.12.1" and then for
"libclntsh.dylib.11.1" before returning an error.


Instant Client Zip
++++++++++++++++++

To run ODPI-C applications with Instant Client zip files:

1. Download the "Basic" or "Basic Light" zip file from `here
<http://www.oracle.com/technetwork/topics/intel-macsoft-096467.html>`__.

2. Unzip the package into a single directory that is accessible to your
   application. For example::

       mkdir -p /opt/oracle
       unzip instantclient-basic-macos.x64-12.1.0.2.0.zip

4. Add links to $HOME/lib to enable applications to find the library. For
   example::

       cd $HOME/lib
       ln -s /opt/oracle/instantclient_12_1/libclntsh.dylib.12.1

   Alternatively, copy the required OCI libraries. For example::

        mkdir ~/lib
        cp /opt/oracle/instantclient_12_1/{libclntsh.dylib.12.1,libclntshcore.dylib.12.1,libons.dylib,libnnz12.dylib,libociei.dylib} ~/lib/

   For Instant Client 11.2, the OCI libraries must be copied. For example::

        mkdir ~/lib
        cp /opt/oracle/instantclient_11_2/{libclntsh.dylib.11.1,libnnz11.dylib,libociei.dylib} ~/lib/


Other Platforms
===============

To run ODPI-C applications on other platforms (such as Solaris and AIX), follow the same
general directions as for Linux Instant Client zip files or Local Database.
